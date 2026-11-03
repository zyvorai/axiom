package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/zyvorai/axiom/internal/auth"
	"github.com/zyvorai/axiom/internal/platform"
)

type AuthOptions struct {
	APIKey string
	// SessionSecret signs OIDC session cookies/tokens. Left empty, NewWithUsers
	// generates a random per-process secret rather than falling back to the
	// Basic-auth password — reusing the password would let anyone who knows
	// it forge an admin session and bypass OIDC entirely. Set
	// AXIOM_SESSION_SECRET for sessions that survive a restart or are shared
	// across replicas.
	SessionSecret string
	// TrustedProxyHeader, when set, tells the login rate limiter to key on
	// this header (e.g. "X-Forwarded-For") instead of the raw connection
	// address. Only set this when Axiom sits behind a reverse proxy that
	// itself sets/overwrites the header — see auth.Guard's doc comment.
	TrustedProxyHeader string
	// RevocationStore persists session revocations across restarts. Optional —
	// nil keeps them in memory.
	RevocationStore auth.RevocationStore
	// Revocations, when set, is used instead of building one from
	// RevocationStore, so the caller can run its cross-replica refresh
	// (auth.Revocations.StartRefresh). Optional.
	Revocations *auth.Revocations
	// RequireMFA (AXIOM_REQUIRE_MFA) is which local accounts must enrol a
	// second factor before they can use the console. Zero value: off.
	RequireMFA auth.MFAPolicy
	// HelmRepoStore persists the Helm chart catalog's repository list.
	// Optional — nil keeps it in memory.
	HelmRepoStore platform.HelmRepoStore
	// CostBudgetStore persists per-namespace cost budgets. Optional — nil
	// keeps them in memory.
	CostBudgetStore platform.CostBudgetStore
	// TOTPSecret, when set (AXIOM_TOTP_SECRET), adds a TOTP second factor to the
	// built-in admin account. Base32, at least 80 bits.
	TOTPSecret string
	OIDCLogin  string
	OIDCIssuer         string
	OIDCClient         string
	SAMLLogin          string
	SAMLEntityID       string
	SAMLIDPCert        string
	SAMLAllowUnsigned  bool
	LDAPEnabled        bool
	LDAPDomain         string
	LDAPURL            string
	LDAPBaseDN         string
	LDAPBindDN         string
	LDAPBindPW         string
	LDAPUserAttr       string
	// LDAPGroupAttr/LDAPAdminGroup/LDAPViewerGroup map a bound user's directory
	// group membership to a viewer/admin/operator role, the LDAP equivalent of
	// OIDCGroupsClaim/OIDCAdminGroup/OIDCViewerGroup below. Group lookup only
	// works for the BaseDN-constructed bind form (auth.ldapBindDN's "uid=...,
	// BaseDN" case) — see auth.LDAPAuthenticate. When both groups are unset,
	// every successful bind keeps the long-standing "operator" default.
	LDAPGroupAttr   string
	LDAPAdminGroup  string
	LDAPViewerGroup string

	// OIDCGroupsClaim/OIDCAdminGroup/OIDCViewerGroup map an ID token's groups
	// claim to a viewer/admin/operator role. When OIDCAdminGroup and
	// OIDCViewerGroup are both unset, OIDCDefaultRole is used (default admin
	// for backwards compatibility — set to viewer for safer greenfield).
	OIDCGroupsClaim  string
	OIDCAdminGroup   string
	OIDCViewerGroup  string
	OIDCDefaultRole  string

	// SAMLAllowIdPInitiated accepts SAML responses with no InResponseTo
	// (AXIOM_SAML_ALLOW_IDP_INITIATED). Off by default: an unsolicited
	// response cannot be bound to a sign-in this browser started.
	SAMLAllowIdPInitiated bool

	// PodProxy, when set (AXIOM_POD_PROXY_ADDR / AXIOM_POD_PROXY_ORIGIN), serves
	// pod windows from a separate origin; see podproxy_origin.go. Optional.
	PodProxy *PodProxyOrigin
}

type Server struct {
	provider    platform.Provider
	mode        string
	username    string
	password    string
	allowOrigin string
	auth        AuthOptions
	hub         *platform.IntegrationHub
	users       *platform.UserHub
	tickets     *auth.TicketStore // single-use ws/SSE tickets (see tickets.go)
	podProxy    *PodProxyOrigin   // separate pod-proxy origin, or nil (podproxy_origin.go)
	revocations *auth.Revocations // revoked sessions (logout, sign out everywhere)
	envTOTP     *auth.EnvTOTP     // second factor for the env account, or nil
	guard       auth.Guard        // used to re-verify a password for account-security actions
	aiRL        aiRateLimiter // per-user limiter for /ai/explain (zero value ready)

	samlReqs samlRequestStore // outstanding SAML AuthnRequest IDs (saml.go; zero value ready)
}

func New(provider platform.Provider, mode, username, password, allowOrigin string, authOpts ...AuthOptions) http.Handler {
	return NewWithHub(provider, mode, username, password, allowOrigin, nil, authOpts...)
}

func NewWithAtlas(provider platform.Provider, mode, username, password, allowOrigin string, atlas *platform.AtlasClient, authOpts ...AuthOptions) http.Handler {
	hub := platform.NewIntegrationHub(platform.Integrations{}, nil)
	if atlas != nil && atlas.Enabled() {
		_, _ = hub.Update(context.Background(), platform.IntegrationsPatch{
			AtlasURL:   strPtrAPI(atlas.BaseURL),
			AtlasToken: strPtrAPI(atlas.Token),
		})
	}
	return NewWithHub(provider, mode, username, password, allowOrigin, hub, authOpts...)
}

func NewWithHub(provider platform.Provider, mode, username, password, allowOrigin string, hub *platform.IntegrationHub, authOpts ...AuthOptions) http.Handler {
	return NewWithUsers(provider, mode, username, password, allowOrigin, hub, nil, authOpts...)
}

func NewWithUsers(provider platform.Provider, mode, username, password, allowOrigin string, hub *platform.IntegrationHub, users *platform.UserHub, authOpts ...AuthOptions) http.Handler {
	var opts AuthOptions
	if len(authOpts) > 0 {
		opts = authOpts[0]
	}
	if opts.SessionSecret == "" {
		// A random per-process secret is a safer default than reusing the
		// Basic-auth password: OIDC sessions won't survive a restart, but
		// nothing lets an attacker who knows the login password forge one.
		opts.SessionSecret = auth.RandomURLToken(32)
	}
	if hub != nil {
		cur := hub.Get()
		if opts.OIDCIssuer == "" {
			opts.OIDCIssuer = cur.OIDCIssuer
		}
		if opts.OIDCLogin == "" {
			opts.OIDCLogin = cur.OIDCLoginURL
		}
		if opts.OIDCClient == "" {
			opts.OIDCClient = cur.OIDCClientID
		}
	}
	revocations := opts.Revocations
	if revocations == nil {
		revocations = auth.NewRevocations(opts.RevocationStore)
	}
	s := &Server{revocations: revocations, provider: provider, mode: mode, username: username, password: password, allowOrigin: allowOrigin, auth: opts, hub: hub, users: users, tickets: auth.NewTicketStore()}
	if opts.PodProxy != nil {
		opts.PodProxy.attach(provider, revocationsChecker{revocations})
		s.podProxy = opts.PodProxy
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", s.health)
	mux.HandleFunc("GET /api/v1/metrics", s.metrics)
	mux.HandleFunc("GET /api/v1/session", s.session)
	mux.HandleFunc("GET /api/v1/overview", s.overview)
	mux.HandleFunc("GET /api/v1/workloads", s.workloads)
	mux.HandleFunc("POST /api/v1/workloads", s.launch)
	mux.HandleFunc("POST /api/v1/workloads/{namespace}/{kind}/{name}/scale", s.scale)
	mux.HandleFunc("POST /api/v1/workloads/{namespace}/{kind}/{name}/start", s.start)
	mux.HandleFunc("POST /api/v1/workloads/{namespace}/{kind}/{name}/stop", s.stop)
	mux.HandleFunc("POST /api/v1/workloads/{namespace}/{kind}/{name}/restart", s.restart)
	mux.HandleFunc("DELETE /api/v1/workloads/{namespace}/{kind}/{name}", s.deleteWorkload)
	mux.HandleFunc("GET /api/v1/pods", s.pods)
	mux.HandleFunc("DELETE /api/v1/pods/{namespace}/{name}", s.deletePod)
	mux.HandleFunc("POST /api/v1/pods/{namespace}/{name}/evict", s.evictPod)
	mux.HandleFunc("GET /api/v1/pods/{namespace}/{name}/diagnosis", s.diagnosePod)
	mux.HandleFunc("GET /api/v1/pods/{namespace}/{name}/logs", s.logs)
	mux.HandleFunc("POST /api/v1/pods/{namespace}/{name}/exec", s.execCommand)
	mux.HandleFunc("GET /api/v1/nodes", s.nodes)
	mux.HandleFunc("GET /api/v1/vms", s.vms)
	mux.HandleFunc("POST /api/v1/vms", s.createVM)
	mux.HandleFunc("DELETE /api/v1/vms/{namespace}/{name}", s.deleteVM)
	mux.HandleFunc("POST /api/v1/vms/{namespace}/{name}/start", s.startVM)
	mux.HandleFunc("POST /api/v1/vms/{namespace}/{name}/stop", s.stopVM)
	mux.HandleFunc("GET /api/v1/volumes", s.volumes)
	mux.HandleFunc("POST /api/v1/volumes", s.createVolume)
	mux.HandleFunc("DELETE /api/v1/volumes/{namespace}/{name}", s.deleteVolume)
	mux.HandleFunc("GET /api/v1/metrics/utilization", s.utilization)
	mux.HandleFunc("GET /api/v1/regions", s.regions)
	mux.HandleFunc("POST /api/v1/nodes/{name}/cordon", s.cordon)
	mux.HandleFunc("POST /api/v1/nodes/{name}/uncordon", s.uncordon)
	mux.HandleFunc("POST /api/v1/nodes/{name}/drain", s.drain)
	mux.HandleFunc("GET /api/v1/nodes/{name}/metrics", s.nodeMetrics)
	mux.HandleFunc("POST /api/v1/nodes/{name}/taints", s.addTaint)
	mux.HandleFunc("DELETE /api/v1/nodes/{name}/taints/{key}", s.removeTaint)
	mux.HandleFunc("GET /api/v1/events", s.events)
	mux.HandleFunc("GET /api/v1/namespaces", s.namespaces)
	mux.HandleFunc("POST /api/v1/namespaces", s.createNamespace)
	mux.HandleFunc("DELETE /api/v1/namespaces/{name}", s.deleteNamespace)
	mux.HandleFunc("GET /api/v1/services", s.services)
	mux.HandleFunc("POST /api/v1/services", s.createService)
	mux.HandleFunc("DELETE /api/v1/services/{namespace}/{name}", s.deleteService)
	mux.HandleFunc("GET /api/v1/network", s.network)
	mux.HandleFunc("POST /api/v1/intent/parse", s.parseIntent)
	mux.HandleFunc("POST /api/v1/intent/place", s.placeIntent)
	mux.HandleFunc("GET /api/v1/workloads/{namespace}/{kind}/{name}/still", s.stillPreview)
	mux.HandleFunc("POST /api/v1/workloads/{namespace}/{kind}/{name}/seal", s.sealWorkload)
	mux.HandleFunc("POST /api/v1/workloads/{namespace}/{kind}/{name}/unseal", s.unsealWorkload)
	mux.HandleFunc("GET /api/v1/pods/{namespace}/{name}/window/{port}/", s.podWindow)
	mux.HandleFunc("POST /api/v1/pods/{namespace}/{name}/window-link", s.podWindowLink)
	mux.HandleFunc("GET /api/v1/auth/providers", s.authProviders)
	mux.HandleFunc("GET /api/v1/auth/oidc/login", s.oidcLogin)
	mux.HandleFunc("GET /api/v1/auth/oidc/callback", s.oidcCallback)
	mux.HandleFunc("POST /api/v1/auth/logout", s.authLogout)
	s.registerIdentity(mux)
	s.registerFleet(mux)
	s.registerFleetSearch(mux)
	s.registerGitOpsCreate(mux)
	s.registerGitOpsDelete(mux)
	s.registerHelmCatalog(mux)
	s.registerAutoscaling(mux)
	s.registerCost(mux)
	mux.HandleFunc("GET /api/v1/auth/saml/login", s.samlLogin)
	mux.HandleFunc("POST /api/v1/auth/saml/acs", s.samlACS)
	mux.HandleFunc("GET /api/v1/nodes/{name}", s.nodeDetail)
	mux.HandleFunc("GET /api/v1/workloads/{namespace}/{kind}/{name}/rollout", s.rolloutStatus)
	mux.HandleFunc("POST /api/v1/workloads/{namespace}/{kind}/{name}/rollout/{action}", s.rolloutAction)
	mux.HandleFunc("GET /api/v1/resources/{namespace}/{kind}/{name}", s.resourceDetail)
	mux.HandleFunc("GET /api/v1/workloads/{namespace}/{kind}/{name}/network/guard", s.workloadNetworkGuard)
	mux.HandleFunc("GET /api/v1/packetwolf/status", s.packetwolfStatus)
	mux.HandleFunc("GET /api/v1/packetwolf/proxy/{path...}", s.packetwolfProxy)

	mux.HandleFunc("GET /api/v1/secrets", s.listSecrets)
	mux.HandleFunc("POST /api/v1/secrets", s.createSecret)
	mux.HandleFunc("PUT /api/v1/secrets/{namespace}/{name}", s.updateSecret)
	mux.HandleFunc("GET /api/v1/secrets/{namespace}/{name}", s.getSecret)
	mux.HandleFunc("DELETE /api/v1/secrets/{namespace}/{name}", s.deleteSecret)
	mux.HandleFunc("GET /api/v1/configmaps", s.listConfigMaps)
	mux.HandleFunc("POST /api/v1/configmaps", s.createConfigMap)
	mux.HandleFunc("PUT /api/v1/configmaps/{namespace}/{name}", s.updateConfigMap)
	mux.HandleFunc("GET /api/v1/configmaps/{namespace}/{name}", s.getConfigMap)
	mux.HandleFunc("DELETE /api/v1/configmaps/{namespace}/{name}", s.deleteConfigMap)
	mux.HandleFunc("GET /api/v1/resourcequotas", s.listResourceQuotas)
	mux.HandleFunc("POST /api/v1/resourcequotas", s.createResourceQuota)
	mux.HandleFunc("DELETE /api/v1/resourcequotas/{namespace}/{name}", s.deleteResourceQuota)
	mux.HandleFunc("GET /api/v1/limitranges", s.listLimitRanges)
	mux.HandleFunc("GET /api/v1/ingresses", s.listIngresses)
	mux.HandleFunc("POST /api/v1/ingresses", s.createIngress)
	mux.HandleFunc("DELETE /api/v1/ingresses/{namespace}/{name}", s.deleteIngress)
	mux.HandleFunc("GET /api/v1/clusters", s.listClusters)
	mux.HandleFunc("POST /api/v1/clusters", s.registerCluster)
	mux.HandleFunc("DELETE /api/v1/clusters/{name}", s.unregisterCluster)
	mux.HandleFunc("POST /api/v1/clusters/{name}/activate", s.activateCluster)
	mux.HandleFunc("GET /api/v1/jobs", s.listJobs)
	mux.HandleFunc("DELETE /api/v1/jobs/{namespace}/{name}", s.deleteJob)
	mux.HandleFunc("GET /api/v1/cronjobs", s.listCronJobs)
	mux.HandleFunc("POST /api/v1/cronjobs/{namespace}/{name}/trigger", s.triggerCronJob)
	mux.HandleFunc("POST /api/v1/cronjobs/{namespace}/{name}/suspend", s.suspendCronJob)
	mux.HandleFunc("DELETE /api/v1/cronjobs/{namespace}/{name}", s.deleteCronJob)
	mux.HandleFunc("GET /api/v1/hpas", s.listHPAs)
	mux.HandleFunc("POST /api/v1/hpas", s.createHPA)
	mux.HandleFunc("DELETE /api/v1/hpas/{namespace}/{name}", s.deleteHPA)
	mux.HandleFunc("GET /api/v1/poddisruptionbudgets", s.listPodDisruptionBudgets)
	mux.HandleFunc("GET /api/v1/networkpolicies", s.listNetworkPolicies)
	mux.HandleFunc("DELETE /api/v1/networkpolicies/{namespace}/{name}", s.deleteNetworkPolicy)
	mux.HandleFunc("POST /api/v1/apply", s.applyManifest)
	mux.HandleFunc("POST /api/v1/apply/diff", s.diffManifest)
	mux.HandleFunc("GET /api/v1/graph/{namespace}/{kind}/{name}", s.resourceGraph)
	mux.HandleFunc("GET /api/v1/pods/{namespace}/{name}/metrics", s.podMetrics)
	mux.HandleFunc("GET /api/v1/crds", s.listCRDs)
	mux.HandleFunc("GET /api/v1/crds/{group}/{version}/{plural}", s.listCRDInstances)
	mux.HandleFunc("GET /api/v1/crds/{group}/{version}/{plural}/{name}", s.getCRDInstance)
	mux.HandleFunc("DELETE /api/v1/crds/{group}/{version}/{plural}/{name}", s.deleteCRDInstance)
	mux.HandleFunc("GET /api/v1/plugins", s.listPlugins)
	mux.HandleFunc("GET /api/v1/admission-webhooks", s.listAdmissionWebhooks)
	mux.HandleFunc("GET /api/v1/storageclasses", s.listStorageClasses)
	mux.HandleFunc("GET /api/v1/priorityclasses", s.listPriorityClasses)
	mux.HandleFunc("GET /api/v1/persistentvolumes", s.listPersistentVolumes)
	mux.HandleFunc("GET /api/v1/gateways", s.listGateways)
	mux.HandleFunc("GET /api/v1/backups", s.listBackups)
	mux.HandleFunc("POST /api/v1/backups", s.createBackup)
	mux.HandleFunc("POST /api/v1/backups/restore", s.restoreBackup)
	mux.HandleFunc("GET /api/v1/helm/releases", s.helmReleases)
	mux.HandleFunc("GET /api/v1/helm/releases/{namespace}/{name}/history", s.helmHistory)
	mux.HandleFunc("POST /api/v1/helm/releases/{namespace}/{name}/uninstall", s.helmUninstall)
	mux.HandleFunc("POST /api/v1/helm/releases/{namespace}/{name}/rollback", s.helmRollback)
	mux.HandleFunc("POST /api/v1/helm/install", s.helmInstall)
	mux.HandleFunc("POST /api/v1/helm/upgrade", s.helmUpgrade)
	mux.HandleFunc("GET /api/v1/external-secrets", s.externalSecrets)
	mux.HandleFunc("GET /api/v1/vm-snapshots", s.vmSnapshots)
	mux.HandleFunc("POST /api/v1/vm-snapshots", s.createVMSnapshot)
	mux.HandleFunc("POST /api/v1/vms/{namespace}/{name}/migrate", s.migrateVM)
	mux.HandleFunc("GET /api/v1/vm-clones", s.vmClones)
	mux.HandleFunc("POST /api/v1/vm-clones", s.createVMClone)
	mux.HandleFunc("GET /api/v1/vm-exports", s.vmExports)
	mux.HandleFunc("POST /api/v1/vm-exports", s.createVMExport)
	mux.HandleFunc("GET /api/v1/vm-restores", s.vmRestores)
	mux.HandleFunc("POST /api/v1/vm-restores", s.createVMRestore)
	mux.HandleFunc("GET /api/v1/velero/backups", s.veleroBackups)
	mux.HandleFunc("POST /api/v1/velero/backups", s.createVeleroBackup)
	mux.HandleFunc("GET /api/v1/velero/schedules", s.veleroSchedules)
	mux.HandleFunc("POST /api/v1/velero/schedules", s.createVeleroSchedule)
	mux.HandleFunc("GET /api/v1/gitops", s.gitops)
	mux.HandleFunc("POST /api/v1/gitops/{kind}/{namespace}/{name}/sync", s.triggerGitOpsSync)
	mux.HandleFunc("GET /api/v1/alerts", s.alerts)
	mux.HandleFunc("GET /api/v1/audit", s.audit)
	mux.HandleFunc("GET /api/v1/rbac", s.rbac)
	mux.HandleFunc("POST /api/v1/rbac/roles", s.createRole)
	mux.HandleFunc("DELETE /api/v1/rbac/roles/{namespace}/{name}", s.deleteRole)
	mux.HandleFunc("POST /api/v1/rbac/rolebindings", s.createRoleBinding)
	mux.HandleFunc("DELETE /api/v1/rbac/rolebindings/{namespace}/{name}", s.deleteRoleBinding)
	mux.HandleFunc("GET /api/v1/cost", s.cost)
	mux.HandleFunc("POST /api/v1/validate", s.validateManifest)
	mux.HandleFunc("GET /api/v1/vms/{namespace}/{name}/console", s.vmConsole)
	mux.HandleFunc("GET /api/v1/vms/{namespace}/{name}/vnc", s.vmVNC)

	mux.HandleFunc("GET /api/v1/agent-runs", s.listAgentRuns)
	mux.HandleFunc("POST /api/v1/agent-runs", s.createAgentRun)
	mux.HandleFunc("GET /api/v1/agent-runs/{namespace}/{name}", s.getAgentRun)
	mux.HandleFunc("POST /api/v1/agent-runs/{namespace}/{name}/approve", s.approveAgentRun)
	mux.HandleFunc("POST /api/v1/agent-runs/{namespace}/{name}/deny", s.denyAgentRun)
	mux.HandleFunc("GET /api/v1/platform-intents", s.listPlatformIntents)
	mux.HandleFunc("GET /api/v1/haven/identity", s.havenIdentity)
	mux.HandleFunc("GET /api/v1/haven/planes", s.listIdentityPlanes)
	mux.HandleFunc("GET /api/v1/haven/admin", s.havenAdmin)
	mux.HandleFunc("GET /api/v1/atlas", s.atlasSummary)
	mux.HandleFunc("GET /api/v1/atlas/{path...}", s.atlasProxy)
	mux.HandleFunc("POST /api/v1/atlas/volumes", s.atlasCreateVolume)
	mux.HandleFunc("GET /api/v1/integrations", s.getIntegrations)
	mux.HandleFunc("GET /api/v1/integrations/examples", s.getIntegrationExamples)
	mux.HandleFunc("PUT /api/v1/integrations", s.putIntegrations)
	mux.HandleFunc("POST /api/v1/integrations/atlas/test", s.testAtlas)
	mux.HandleFunc("POST /api/v1/integrations/oidc/test", s.testOIDC)
	mux.HandleFunc("POST /api/v1/integrations/postgres/test", s.testPostgres)
	mux.HandleFunc("POST /api/v1/integrations/probe/test", s.testProbe)
	mux.HandleFunc("POST /api/v1/integrations/slack/test", s.testSlackNotify)
	mux.HandleFunc("POST /api/v1/integrations/webhook/test", s.testAlertWebhook)
	// AI assistant (opt-in; see internal/api/ai.go)
	mux.HandleFunc("POST /api/v1/integrations/ai/test", s.testAI)
	mux.HandleFunc("GET /api/v1/ai/status", s.aiStatus)
	mux.HandleFunc("POST /api/v1/ai/explain", s.aiExplain)
	mux.HandleFunc("GET /api/v1/users", s.listUsers)
	mux.HandleFunc("POST /api/v1/users", s.createUser)
	mux.HandleFunc("PATCH /api/v1/users/{username}", s.updateUser)
	mux.HandleFunc("DELETE /api/v1/users/{username}", s.deleteUser)

	// Troubleshooting & timeline (Workstream B): routes live in troubleshoot.go.
	s.registerTroubleshooting(mux)
	// Security views + auth hardening (Workstream C): RBAC viewer, NetworkPolicy
	// analysis, Trivy/Kyverno report aggregation, stream tickets.
	s.registerSecurityRoutes(mux)

	RegisterSSE(mux)
	RegisterRealtime(mux, provider, s.podProxyEnabled)

	var authenticator auth.UserAuthenticator
	if users != nil {
		authenticator = users
	}
	var mfa auth.MFAProvider
	if users != nil {
		mfa = users
		// Anything that should end a person's sessions (password, role,
		// namespaces, MFA changes, deletion) revokes them.
		users.SetInvalidateHook(s.revocations.RevokeUser)
	}
	if et, ok := auth.NewEnvTOTP(opts.TOTPSecret); !ok {
		log.Printf("auth: AXIOM_TOTP_SECRET is not a valid base32 secret (at least 16 characters); ignoring it")
	} else {
		s.envTOTP = et
	}
	guard := auth.Guard{
		Username: username, Password: password, APIKey: opts.APIKey,
		MFA: mfa, EnvTOTP: s.envTOTP, Revocations: s.revocations, RequireMFA: opts.RequireMFA,
		SessionSecret: opts.SessionSecret, Users: authenticator, Tickets: s.tickets,
		Audit: axiomAuditLogger{}, TrustedProxyHeader: opts.TrustedProxyHeader,
		LDAP: auth.LDAPConfig{
			Enabled: opts.LDAPEnabled, URL: opts.LDAPURL, Domain: opts.LDAPDomain,
			BaseDN: opts.LDAPBaseDN, BindDN: opts.LDAPBindDN, BindPW: opts.LDAPBindPW, UserAttr: opts.LDAPUserAttr,
			GroupAttr: opts.LDAPGroupAttr, AdminGroup: opts.LDAPAdminGroup, ViewerGroup: opts.LDAPViewerGroup,
		},
	}
	s.guard = guard
	if guard.EnvAdminExemptFromMFA() {
		log.Printf("auth: AXIOM_REQUIRE_MFA=%s but the built-in admin has no AXIOM_TOTP_SECRET; it is exempt so you cannot lock yourself out", opts.RequireMFA)
	}
	// Keep health + auth discovery/redirects public. Everything else stays behind Basic/API-key/OIDC session.
	secured := guard.Middleware(clusterSessionMiddleware(mux))
	return s.common(secured)
}

// clusterSessionMiddleware scopes multi-cluster "active cluster" selection
// (see platform.SwitchableProvider) to the calling browser tab/script
// instead of one process-wide switch that different tabs or users would
// otherwise stomp on. It runs after auth.Guard.Middleware, so the resolved
// Identity is already on the request context to use as a fallback key for
// callers that send no session header (e.g. scripted API/curl access).
func clusterSessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get(platform.ClusterSessionHeader)
		if key == "" {
			if id, ok := auth.IdentityFromContext(r.Context()); ok && id.User != "" {
				key = "user:" + id.User
			}
		}
		if key != "" {
			r = r.WithContext(platform.WithClusterSession(r.Context(), key))
		}
		next.ServeHTTP(w, r)
	})
}

// axiomAuditLogger adapts platform.RecordAxiomAudit to auth.AuditLogger, so
// the auth package can record login failures/lockouts without importing
// platform directly.
type axiomAuditLogger struct{}

func (axiomAuditLogger) Record(action, target, detail, user string) {
	platform.RecordAxiomAudit(action, target, detail, user)
}

func strPtrAPI(s string) *string { return &s }

func (s *Server) atlasClient() *platform.AtlasClient {
	if s.hub != nil {
		return s.hub.Atlas()
	}
	return nil
}

func (s *Server) common(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if s.requestSecure(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		if s.allowOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", s.allowOrigin)
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, "+platform.ClusterSessionHeader)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		elapsed := time.Since(start)
		globalMetrics.record(r.Method, r.URL.Path, rec.status, elapsed.Seconds())
		log.Printf("%s %q %s", sanitizeMethod(r.Method), r.URL.Path, elapsed.Round(time.Millisecond))
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "mode": s.mode, "time": time.Now().UTC()})
}
func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	cluster := "zyvor"
	if s.mode == "demo" {
		cluster = "zyvor-demo"
	}
	user := s.username
	role := "admin"
	authMethod := "local"
	if sess, ok := auth.SessionFromRequest(s.sessionSecret(), r); ok {
		user = sess.User
		authMethod = "oidc"
		if sess.Issuer == auth.LocalSessionIssuer {
			authMethod = "local"
		}
		if sess.Role != "" {
			role = sess.Role
		}
	} else if id, ok := auth.IdentityFromContext(r.Context()); ok {
		user = id.User
		if id.Role != "" {
			role = id.Role
		}
	} else if u, _, ok := r.BasicAuth(); ok && u != "" {
		user = u
	}
	roleLabel := "Platform Admin"
	if role == "viewer" {
		roleLabel = "Viewer"
	}
	regions := platform.DefaultRegions(cluster)
	// mfa.required: the require-MFA policy covers this identity (the UI forces
	// enrolment when it is true and enrolled is false). Always false for SSO,
	// LDAP and API-key identities.
	var mfaRequired, mfaEnrolled bool
	if id, ok := auth.IdentityFromContext(r.Context()); ok {
		mfaRequired, mfaEnrolled = s.guard.MFAState(id)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user": user, "role": roleLabel, "mode": s.mode, "auth": authMethod,
		"cluster": cluster, "region": platform.ActiveRegion(regions), "regions": regions,
		"mfa": map[string]bool{"required": mfaRequired, "enrolled": mfaEnrolled},
	})
}
func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.Overview(r.Context())
	respond(w, v, err)
}
func (s *Server) workloads(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListWorkloads(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}
func (s *Server) launch(w http.ResponseWriter, r *http.Request) {
	var req platform.LaunchRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.provider.Launch(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("workload.launch", req.Namespace+"/"+req.Name, req.Image, s.auditUser(r))
	writeJSON(w, http.StatusCreated, v)
	EmitStateChanged("workload", "create")
}
func (s *Server) scale(w http.ResponseWriter, r *http.Request) {
	var req platform.ScaleRequest
	if err := decode(r, &req); err != nil {
		fail(w, 400, err)
		return
	}
	ns, kind, name := r.PathValue("namespace"), r.PathValue("kind"), r.PathValue("name")
	err := s.provider.Scale(r.Context(), ns, kind, name, req.Replicas)
	if err == nil {
		platform.RecordAxiomAudit("workload.scale", ns+"/"+kind+"/"+name, fmt.Sprintf("replicas=%d", req.Replicas), s.auditUser(r))
		EmitStateChanged("workload", "scale")
	}
	respondAction(w, err)
}
func (s *Server) start(w http.ResponseWriter, r *http.Request) {
	ns, kind, name := r.PathValue("namespace"), r.PathValue("kind"), r.PathValue("name")
	err := s.provider.Start(r.Context(), ns, kind, name)
	if err == nil {
		platform.RecordAxiomAudit("workload.start", ns+"/"+kind+"/"+name, "", s.auditUser(r))
		EmitStateChanged("workload", "start")
	}
	respondAction(w, err)
}
func (s *Server) stop(w http.ResponseWriter, r *http.Request) {
	ns, kind, name := r.PathValue("namespace"), r.PathValue("kind"), r.PathValue("name")
	err := s.provider.Stop(r.Context(), ns, kind, name)
	if err == nil {
		platform.RecordAxiomAudit("workload.stop", ns+"/"+kind+"/"+name, "", s.auditUser(r))
		EmitStateChanged("workload", "stop")
	}
	respondAction(w, err)
}
func (s *Server) restart(w http.ResponseWriter, r *http.Request) {
	ns, kind, name := r.PathValue("namespace"), r.PathValue("kind"), r.PathValue("name")
	err := s.provider.Restart(r.Context(), ns, kind, name)
	if err == nil {
		platform.RecordAxiomAudit("workload.restart", ns+"/"+kind+"/"+name, "", s.auditUser(r))
		EmitStateChanged("workload", "restart")
	}
	respondAction(w, err)
}
func (s *Server) deleteWorkload(w http.ResponseWriter, r *http.Request) {
	ns, kind, name := r.PathValue("namespace"), r.PathValue("kind"), r.PathValue("name")
	err := s.provider.DeleteWorkload(r.Context(), ns, kind, name)
	if err == nil {
		platform.RecordAxiomAudit("workload.delete", ns+"/"+kind+"/"+name, "", s.auditUser(r))
		EmitStateChanged("workload", "delete")
	}
	respondAction(w, err)
}
func (s *Server) pods(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListPods(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}
func (s *Server) deletePod(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	err := s.provider.DeletePod(r.Context(), ns, name)
	if err == nil {
		platform.RecordAxiomAudit("pod.delete", ns+"/"+name, "", s.auditUser(r))
		EmitStateChanged("pod", "delete")
	}
	respondAction(w, err)
}
func (s *Server) evictPod(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	err := s.provider.EvictPod(r.Context(), ns, name)
	if err == nil {
		platform.RecordAxiomAudit("pod.evict", ns+"/"+name, "", s.auditUser(r))
		EmitStateChanged("pod", "evict")
	}
	respondAction(w, err)
}
func (s *Server) diagnosePod(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.DiagnosePod(r.Context(), r.PathValue("namespace"), r.PathValue("name"))
	respond(w, v, err)
}
func (s *Server) nodes(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListNodes(r.Context())
	respond(w, v, err)
}
func (s *Server) vms(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListVirtualMachines(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}
func (s *Server) startVM(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	err := s.provider.StartVirtualMachine(r.Context(), ns, name)
	if err == nil {
		platform.RecordAxiomAudit("vm.start", ns+"/"+name, "", s.auditUser(r))
		EmitStateChanged("vm", "start")
	}
	respondAction(w, err)
}
func (s *Server) stopVM(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	err := s.provider.StopVirtualMachine(r.Context(), ns, name)
	if err == nil {
		platform.RecordAxiomAudit("vm.stop", ns+"/"+name, "", s.auditUser(r))
		EmitStateChanged("vm", "stop")
	}
	respondAction(w, err)
}
func (s *Server) volumes(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListVolumes(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}
func (s *Server) utilization(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.Utilization(r.Context())
	respond(w, v, err)
}
func (s *Server) regions(w http.ResponseWriter, r *http.Request) {
	cluster := "zyvor"
	if s.mode == "demo" {
		cluster = "zyvor-demo"
	}
	respond(w, platform.DefaultRegions(cluster), nil)
}
func (s *Server) cordon(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	err := s.provider.Cordon(r.Context(), name, true)
	if err == nil {
		platform.RecordAxiomAudit("node.cordon", name, "", s.auditUser(r))
		EmitStateChanged("node", "cordon")
	}
	respondAction(w, err)
}
func (s *Server) uncordon(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	err := s.provider.Cordon(r.Context(), name, false)
	if err == nil {
		platform.RecordAxiomAudit("node.uncordon", name, "", s.auditUser(r))
		EmitStateChanged("node", "uncordon")
	}
	respondAction(w, err)
}
func (s *Server) drain(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	err := s.provider.Drain(r.Context(), name)
	if err == nil {
		platform.RecordAxiomAudit("node.drain", name, "", s.auditUser(r))
		EmitStateChanged("node", "drain")
	}
	respondAction(w, err)
}
func (s *Server) nodeMetrics(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.GetNodeMetrics(r.Context(), r.PathValue("name"))
	respond(w, v, err)
}
func (s *Server) addTaint(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key    string `json:"key"`
		Value  string `json:"value"`
		Effect string `json:"effect"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	name := r.PathValue("name")
	err := s.provider.AddTaint(r.Context(), name, req.Key, req.Value, req.Effect)
	if err == nil {
		platform.RecordAxiomAudit("node.taint", name, req.Key+"="+req.Value+":"+req.Effect, s.auditUser(r))
		EmitStateChanged("node", "taint")
	}
	respondAction(w, err)
}
func (s *Server) removeTaint(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	err := s.provider.RemoveTaint(r.Context(), name, r.PathValue("key"), r.URL.Query().Get("effect"))
	if err == nil {
		platform.RecordAxiomAudit("node.untaint", name, r.PathValue("key"), s.auditUser(r))
		EmitStateChanged("node", "untaint")
	}
	respondAction(w, err)
}
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListEvents(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) namespaces(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListNamespaces(r.Context())
	respond(w, v, err)
}

func (s *Server) createNamespace(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, 400, err)
		return
	}
	v, err := s.provider.CreateNamespace(r.Context(), req.Name)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("namespace.create", req.Name, "", s.auditUser(r))
	EmitStateChanged("namespace", "create")
	writeJSON(w, 201, v)
}

func (s *Server) deleteNamespace(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	err := s.provider.DeleteNamespace(r.Context(), name)
	if err == nil {
		platform.RecordAxiomAudit("namespace.delete", name, "", s.auditUser(r))
		EmitStateChanged("namespace", "delete")
	}
	respondAction(w, err)
}

func (s *Server) services(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListServices(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) network(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("namespace")
	if ns == "" {
		ns = "all"
	}
	v, err := s.provider.NetworkMap(r.Context(), ns)
	respond(w, v, err)
}

func (s *Server) parseIntent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Intent string `json:"intent"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, 400, err)
		return
	}
	draft := platform.ParseIntent(req.Intent)
	writeJSON(w, 200, platform.IntentParseResponse{
		Draft: draft, LaunchRequest: draft.LaunchRequest(), Receipt: draft.ReceiptYAML(),
	})
}

func (s *Server) placeIntent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Intent string `json:"intent"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, 400, err)
		return
	}
	draft := platform.ParseIntent(req.Intent)
	launch := draft.LaunchRequest()
	launch.Intent = req.Intent
	v, err := s.provider.Launch(r.Context(), launch)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("workload.launch", launch.Namespace+"/"+launch.Name, "intent", s.auditUser(r))
	EmitStateChanged("workload", "create")
	writeJSON(w, 201, v)
}

func (s *Server) stillPreview(w http.ResponseWriter, r *http.Request) {
	action := platform.StillAction(r.URL.Query().Get("action"))
	if action == "" {
		action = platform.StillHush
	}
	v, err := platform.StillPreview(r.Context(), s.provider, r.PathValue("namespace"), r.PathValue("kind"), r.PathValue("name"), action, r.URL.Query().Get("pod"))
	respond(w, v, err)
}

func (s *Server) sealWorkload(w http.ResponseWriter, r *http.Request) {
	ns, kind, name := r.PathValue("namespace"), r.PathValue("kind"), r.PathValue("name")
	err := s.provider.SealWorkload(r.Context(), ns, kind, name)
	if err == nil {
		platform.RecordAxiomAudit("workload.seal", ns+"/"+kind+"/"+name, "", s.auditUser(r))
		EmitStateChanged("workload", "seal")
	}
	respondAction(w, err)
}

func (s *Server) unsealWorkload(w http.ResponseWriter, r *http.Request) {
	ns, kind, name := r.PathValue("namespace"), r.PathValue("kind"), r.PathValue("name")
	err := s.provider.UnsealWorkload(r.Context(), ns, kind, name)
	if err == nil {
		platform.RecordAxiomAudit("workload.unseal", ns+"/"+kind+"/"+name, "", s.auditUser(r))
		EmitStateChanged("workload", "unseal")
	}
	respondAction(w, err)
}

func (s *Server) podWindow(w http.ResponseWriter, r *http.Request) {
	if s.podProxy != nil {
		// Serving pod content from the console's origin would defeat the
		// separate pod-proxy origin, so the in-origin route is gone.
		fail(w, http.StatusGone, fmt.Errorf("pod windows are served from a separate origin: POST /api/v1/pods/{namespace}/{name}/window-link"))
		return
	}
	port, err := strconv.ParseInt(r.PathValue("port"), 10, 32)
	if err != nil || port <= 0 {
		fail(w, 400, fmt.Errorf("invalid port"))
		return
	}
	target, err := s.provider.PodProxyTarget(r.Context(), r.PathValue("namespace"), r.PathValue("name"), int32(port))
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.IncWindow(r.PathValue("namespace"), r.PathValue("name"))
	proxyPodWindow(w, r, target, r.PathValue("namespace"), r.PathValue("name"), port)
}

func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	tail := int64(200)
	if v := r.URL.Query().Get("tail"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			tail = n
		}
	}
	follow := r.URL.Query().Get("follow") == "true"
	previous := r.URL.Query().Get("previous") == "true"
	stream, err := s.provider.PodLogs(r.Context(), r.PathValue("namespace"), r.PathValue("name"), r.URL.Query().Get("container"), follow, tail, previous)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	defer stream.Close()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(200)
	buf := make([]byte, 32*1024)
	flusher, _ := w.(http.Flusher)
	for {
		n, err := stream.Read(buf)
		if n > 0 {
			_, _ = w.Write(buf[:n])
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Printf("log stream: %v", err)
			}
			return
		}
	}
}

type execRequest struct {
	Container string `json:"container"`
	Command   any    `json:"command"`
}

func (s *Server) execCommand(w http.ResponseWriter, r *http.Request) {
	var req execRequest
	if err := decode(r, &req); err != nil {
		fail(w, 400, err)
		return
	}
	var cmd []string
	switch v := req.Command.(type) {
	case string:
		cmd = []string{"/bin/sh", "-lc", v}
	case []any:
		for _, x := range v {
			cmd = append(cmd, fmt.Sprint(x))
		}
	default:
		fail(w, 400, errors.New("command must be a string or array"))
		return
	}
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	out, err := s.provider.ExecCommand(r.Context(), ns, name, req.Container, cmd)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("pod.exec", ns+"/"+name, strings.Join(cmd, " "), s.auditUser(r))
	writeJSON(w, 200, map[string]string{"output": out})
}

func decode(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
func respond(w http.ResponseWriter, v any, err error) {
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	writeJSON(w, 200, v)
}
func respondAction(w http.ResponseWriter, err error) {
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func fail(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"encode failed"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(b)
	_, _ = w.Write([]byte("\n"))
}

var _ = context.Background
var _ = strings.TrimSpace
