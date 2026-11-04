package auth

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func policyGuard() http.Handler {
	users := &fakeUserAuthenticator{users: map[string]struct {
		password   string
		role       string
		namespaces []string
	}{
		"viewer":  {password: "pw", role: "viewer"},
		"oper":    {password: "pw", role: "operator"},
		"rviewer": {password: "pw", role: "viewer", namespaces: []string{"team-a"}},
		"roper":   {password: "pw", role: "operator", namespaces: []string{"team-a"}},
	}}
	g := Guard{Username: "demo", Password: "demo", Users: users}
	return g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
}

func TestGuardPolicy(t *testing.T) {
	h := policyGuard()
	cases := []struct {
		name, user, method, path string
		want                     int
	}{
		// Viewer must not get interactive sessions through GET/WebSocket routes.
		{"viewer terminal", "viewer", "GET", "/api/v1/pods/ns/p/terminal", 403},
		{"viewer vnc", "viewer", "GET", "/api/v1/vms/ns/v/vnc", 403},
		{"viewer console", "viewer", "GET", "/api/v1/vms/ns/v/console", 403},
		{"viewer pod window proxy", "viewer", "GET", "/api/v1/pods/ns/p/window/8080/", 403},
		// Separate-origin pod proxy link: same guard as the window route.
		{"viewer pod window-link", "viewer", "POST", "/api/v1/pods/ns/p/window-link", 403},
		{"restricted viewer pod window-link", "rviewer", "POST", "/api/v1/pods/team-a/p/window-link", 403},
		{"operator pod window-link", "oper", "POST", "/api/v1/pods/ns/p/window-link", 204},
		{"restricted operator window-link own ns", "roper", "POST", "/api/v1/pods/team-a/p/window-link", 204},
		{"restricted operator window-link other ns", "roper", "POST", "/api/v1/pods/team-b/p/window-link", 403},
		{"restricted operator window-link query smuggle", "roper", "POST", "/api/v1/pods/team-b/p/window-link?namespace=team-a", 403},
		{"admin pod window-link", "demo", "POST", "/api/v1/pods/ns/p/window-link", 204},
		{"viewer logs still allowed", "viewer", "GET", "/api/v1/pods/ns/p/logs", 204},
		{"viewer pod detail still allowed", "viewer", "GET", "/api/v1/pods/ns/p", 204},
		{"operator terminal allowed", "oper", "GET", "/api/v1/pods/ns/p/terminal", 204},
		// Secret values.
		{"viewer secret values", "viewer", "GET", "/api/v1/secrets/ns/s?values=true", 403},
		{"viewer secret decode", "viewer", "GET", "/api/v1/secrets/ns/s?decode=true", 403},
		{"viewer secret metadata ok", "viewer", "GET", "/api/v1/secrets/ns/s", 204},
		{"operator secret values ok", "oper", "GET", "/api/v1/secrets/ns/s?values=true", 204},
		{"operator axiom-system secret values", "oper", "GET", "/api/v1/secrets/axiom-system/zyvor-axiom-remote-clusters?values=true", 403},
		{"admin axiom-system secret values", "demo", "GET", "/api/v1/secrets/axiom-system/zyvor-axiom-remote-clusters?values=true", 204},
		// Operator is not an admin.
		{"operator rbac role create", "oper", "POST", "/api/v1/rbac/roles", 403},
		{"operator rbac read ok", "oper", "GET", "/api/v1/rbac", 204},
		{"operator helm install", "oper", "POST", "/api/v1/helm/install", 403},
		{"operator helm upgrade", "oper", "POST", "/api/v1/helm/upgrade", 403},
		{"operator integration test", "oper", "POST", "/api/v1/integrations/slack/test", 403},
		{"operator integrations read ok", "oper", "GET", "/api/v1/integrations", 204},
		{"operator apply still allowed", "oper", "POST", "/api/v1/apply", 204},
		// YAML diff (dry-run, read-only preview of apply): same gate as apply itself.
		{"viewer apply diff", "viewer", "POST", "/api/v1/apply/diff", 403},
		{"operator apply diff allowed", "oper", "POST", "/api/v1/apply/diff", 204},
		{"admin apply diff allowed", "demo", "POST", "/api/v1/apply/diff", 204},
		{"restricted viewer apply diff", "rviewer", "POST", "/api/v1/apply/diff", 403},
		{"restricted operator apply diff", "roper", "POST", "/api/v1/apply/diff", 403},
		// Resource graph: namespaced read, path namespace authorized even with a permitted ?namespace=.
		{"viewer graph allowed", "viewer", "GET", "/api/v1/graph/ns/Deployment/x", 204},
		{"operator graph allowed", "oper", "GET", "/api/v1/graph/ns/Deployment/x", 204},
		{"restricted graph own namespace", "rviewer", "GET", "/api/v1/graph/team-a/Deployment/x", 204},
		{"restricted graph other namespace", "rviewer", "GET", "/api/v1/graph/team-b/Deployment/x", 403},
		{"restricted graph path/query mismatch", "rviewer", "GET", "/api/v1/graph/team-b/Deployment/x?namespace=team-a", 403},
		{"restricted operator graph other namespace", "roper", "GET", "/api/v1/graph/kube-system/Pod/x", 403},
		{"operator helm rollback allowed", "oper", "POST", "/api/v1/helm/releases/ns/r/rollback", 204},
		{"admin helm install", "demo", "POST", "/api/v1/helm/install", 204},
		// Namespace restriction: path namespace must be authorized even when a
		// permitted ?namespace= is attached.
		{"path/query mismatch", "rviewer", "GET", "/api/v1/secrets/kube-system/x?namespace=team-a", 403},
		{"path allowed + query allowed", "rviewer", "GET", "/api/v1/secrets/team-a/x?namespace=team-a", 204},
		{"query allowed on static subroute", "rviewer", "GET", "/api/v1/events/stream?namespace=team-a", 204},
		{"crd route keeps query semantics", "rviewer", "GET", "/api/v1/crds/apps/v1/deployments?namespace=team-a", 204},
		// Cost budgets: reads for every unrestricted role, writes admin-only;
		// rightsizing is a cluster-wide read; GET /cost is unchanged.
		{"viewer cost budgets read", "viewer", "GET", "/api/v1/cost/budgets", 204},
		{"operator cost budgets read", "oper", "GET", "/api/v1/cost/budgets", 204},
		{"restricted viewer cost budgets read", "rviewer", "GET", "/api/v1/cost/budgets", 403},
		{"restricted operator cost budgets read", "roper", "GET", "/api/v1/cost/budgets", 403},
		{"admin cost budgets read", "demo", "GET", "/api/v1/cost/budgets", 204},
		{"viewer cost budget set", "viewer", "PUT", "/api/v1/cost/budgets/team-a", 403},
		{"operator cost budget set", "oper", "PUT", "/api/v1/cost/budgets/team-a", 403},
		{"restricted viewer cost budget set", "rviewer", "PUT", "/api/v1/cost/budgets/team-a", 403},
		{"restricted operator cost budget set", "roper", "PUT", "/api/v1/cost/budgets/team-a", 403},
		{"admin cost budget set", "demo", "PUT", "/api/v1/cost/budgets/team-a", 204},
		{"viewer cost budget delete", "viewer", "DELETE", "/api/v1/cost/budgets/team-a", 403},
		{"operator cost budget delete", "oper", "DELETE", "/api/v1/cost/budgets/team-a", 403},
		{"restricted viewer cost budget delete", "rviewer", "DELETE", "/api/v1/cost/budgets/team-a", 403},
		{"restricted operator cost budget delete", "roper", "DELETE", "/api/v1/cost/budgets/team-a", 403},
		{"admin cost budget delete", "demo", "DELETE", "/api/v1/cost/budgets/team-a", 204},
		{"viewer rightsizing", "viewer", "GET", "/api/v1/cost/rightsizing", 204},
		{"operator rightsizing", "oper", "GET", "/api/v1/cost/rightsizing?namespace=team-a", 204},
		{"restricted viewer rightsizing", "rviewer", "GET", "/api/v1/cost/rightsizing?namespace=team-a", 403},
		{"restricted operator rightsizing", "roper", "GET", "/api/v1/cost/rightsizing", 403},
		{"admin rightsizing", "demo", "GET", "/api/v1/cost/rightsizing?window=30d", 204},
		// Rightsizing apply changes a workload (like scale): operator and admin;
		// a dry run is a POST too, so the viewer write-block covers it.
		{"viewer rightsizing apply", "viewer", "POST", "/api/v1/cost/rightsizing/apply", 403},
		{"operator rightsizing apply", "oper", "POST", "/api/v1/cost/rightsizing/apply", 204},
		{"restricted viewer rightsizing apply", "rviewer", "POST", "/api/v1/cost/rightsizing/apply", 403},
		{"restricted operator rightsizing apply", "roper", "POST", "/api/v1/cost/rightsizing/apply", 403},
		{"admin rightsizing apply", "demo", "POST", "/api/v1/cost/rightsizing/apply", 204},
		{"restricted viewer cost estimate unchanged", "rviewer", "GET", "/api/v1/cost", 204},
		// Actual-bill reconciliation: a cluster-wide cost read like budgets;
		// configuring and testing it is an admin-only integration.
		{"viewer cost invoice", "viewer", "GET", "/api/v1/cost/invoice", 204},
		{"operator cost invoice", "oper", "GET", "/api/v1/cost/invoice", 204},
		{"admin cost invoice", "demo", "GET", "/api/v1/cost/invoice", 204},
		{"restricted viewer cost invoice", "rviewer", "GET", "/api/v1/cost/invoice", 403},
		{"restricted operator cost invoice", "roper", "GET", "/api/v1/cost/invoice", 403},
		{"restricted operator cost invoice query smuggle", "roper", "GET", "/api/v1/cost/invoice?namespace=team-a", 403},
		{"viewer billing test", "viewer", "POST", "/api/v1/integrations/billing/test", 403},
		{"operator billing test", "oper", "POST", "/api/v1/integrations/billing/test", 403},
		{"restricted operator billing test", "roper", "POST", "/api/v1/integrations/billing/test", 403},
		{"admin billing test", "demo", "POST", "/api/v1/integrations/billing/test", 204},
		{"operator billing settings write", "oper", "PUT", "/api/v1/integrations", 403},
		// Restricted identities cannot reach cluster-wide admin surfaces.
		{"restricted operator apply", "roper", "POST", "/api/v1/apply", 403},
		{"restricted viewer audit", "rviewer", "GET", "/api/v1/audit", 403},
		{"restricted operator delete namespace", "roper", "DELETE", "/api/v1/namespaces/kube-system", 403},
		{"restricted operator helm install", "roper", "POST", "/api/v1/helm/install", 403},
		{"restricted operator own-namespace write", "roper", "POST", "/api/v1/workloads/team-a/Deployment/x/restart", 204},
		{"restricted operator other-namespace write", "roper", "POST", "/api/v1/workloads/team-b/Deployment/x/restart", 403},

		// --- Workstream C: RBAC viewer (read-only; viewers + restricted denied) ---
		{"viewer rbac subjects", "viewer", "GET", "/api/v1/rbac/subjects", 403},
		{"viewer rbac subject detail", "viewer", "GET", "/api/v1/rbac/subjects/User/alice", 403},
		{"viewer rbac who-can", "viewer", "GET", "/api/v1/rbac/who-can?verb=get&resource=secrets", 403},
		{"operator rbac subjects", "oper", "GET", "/api/v1/rbac/subjects", 204},
		{"operator rbac subject detail", "oper", "GET", "/api/v1/rbac/subjects/ServiceAccount/web?namespace=default", 204},
		{"operator rbac who-can", "oper", "GET", "/api/v1/rbac/who-can?verb=get&resource=secrets", 204},
		{"operator still cannot write rbac subjects", "oper", "POST", "/api/v1/rbac/subjects", 403},
		{"operator unknown rbac sub-path still forbidden", "oper", "GET", "/api/v1/rbac/anything-else", 403},
		{"admin rbac subjects", "demo", "GET", "/api/v1/rbac/subjects", 204},
		{"admin rbac who-can", "demo", "GET", "/api/v1/rbac/who-can?verb=list&resource=pods&namespace=team-a", 204},
		{"restricted viewer rbac subjects", "rviewer", "GET", "/api/v1/rbac/subjects", 403},
		{"restricted operator rbac who-can in own ns", "roper", "GET", "/api/v1/rbac/who-can?verb=get&resource=pods&namespace=team-a", 403},
		{"restricted operator rbac subject detail", "roper", "GET", "/api/v1/rbac/subjects/User/x", 403},
		{"viewer service accounts list ok", "viewer", "GET", "/api/v1/serviceaccounts?namespace=team-a", 204},
		{"restricted serviceaccounts own ns", "rviewer", "GET", "/api/v1/serviceaccounts?namespace=team-a", 204},
		{"restricted serviceaccounts other ns", "rviewer", "GET", "/api/v1/serviceaccounts?namespace=team-b", 403},
		{"restricted serviceaccounts all namespaces", "rviewer", "GET", "/api/v1/serviceaccounts", 403},
		// --- NetworkPolicy analysis (namespace in the path) ---
		{"viewer netpol analysis", "viewer", "GET", "/api/v1/networking/policies/team-a/analysis", 204},
		{"restricted netpol analysis own ns", "rviewer", "GET", "/api/v1/networking/policies/team-a/analysis", 204},
		{"restricted netpol analysis other ns", "rviewer", "GET", "/api/v1/networking/policies/team-b/analysis", 403},
		{"restricted netpol analysis path/query mismatch", "rviewer", "GET", "/api/v1/networking/policies/team-b/analysis?namespace=team-a", 403},
		// --- Security reports ---
		{"viewer security reports", "viewer", "GET", "/api/v1/security/reports", 204},
		{"restricted security reports own ns", "rviewer", "GET", "/api/v1/security/reports?namespace=team-a", 204},
		{"restricted security reports no ns", "rviewer", "GET", "/api/v1/security/reports", 403},
		{"restricted security reports other ns", "rviewer", "GET", "/api/v1/security/reports?namespace=team-b", 403},
		{"restricted security drilldown own ns", "rviewer", "GET", "/api/v1/security/reports/team-a/Deployment/web", 204},
		{"restricted security drilldown other ns", "rviewer", "GET", "/api/v1/security/reports/team-b/Deployment/web", 403},
		{"restricted security drilldown mismatch", "rviewer", "GET", "/api/v1/security/reports/team-b/Deployment/web?namespace=team-a", 403},
		// --- Stream tickets: minting is allowed for every authenticated role ---
		{"viewer can mint a ticket", "viewer", "POST", "/api/v1/ws-ticket", 204},
		{"operator can mint a ticket", "oper", "POST", "/api/v1/ws-ticket", 204},
		{"restricted viewer can mint a ticket", "rviewer", "POST", "/api/v1/ws-ticket", 204},
		{"restricted operator can mint a ticket", "roper", "POST", "/api/v1/ws-ticket", 204},
		{"ticket exemption is not a prefix", "viewer", "POST", "/api/v1/ws-ticket/other", 403},
		{"ticket exemption does not cover restricted writes", "roper", "POST", "/api/v1/ws-ticket/other", 403},
		// --- Account security: self-service for every role, admin-only for others' accounts ---
		{"viewer can manage own mfa", "viewer", "POST", "/api/v1/auth/mfa/setup", 204},
		{"viewer can enable mfa", "viewer", "POST", "/api/v1/auth/mfa/enable", 204},
		{"viewer can disable mfa", "viewer", "POST", "/api/v1/auth/mfa/disable", 204},
		{"viewer can sign out everywhere", "viewer", "POST", "/api/v1/auth/logout-all", 204},
		{"restricted viewer can manage own mfa", "rviewer", "POST", "/api/v1/auth/mfa/setup", 204},
		{"restricted operator can sign out everywhere", "roper", "POST", "/api/v1/auth/logout-all", 204},
		{"operator can manage own mfa", "oper", "POST", "/api/v1/auth/mfa/setup", 204},
		{"viewer reads mfa status", "viewer", "GET", "/api/v1/auth/mfa", 204},
		{"self-service exemption is exact", "viewer", "POST", "/api/v1/auth/mfa/other", 403},
		{"self-service exemption is not a prefix", "viewer", "POST", "/api/v1/auth/logout-all/x", 403},
		{"restricted self-service exemption is exact", "roper", "POST", "/api/v1/auth/mfa/other", 403},
		{"operator cannot reset someone's mfa", "oper", "DELETE", "/api/v1/users/viewer/mfa", 403},
		{"operator cannot revoke someone's sessions", "oper", "POST", "/api/v1/users/viewer/revoke-sessions", 403},
		{"viewer cannot reset mfa", "viewer", "DELETE", "/api/v1/users/viewer/mfa", 403},
		{"restricted operator cannot reset mfa", "roper", "DELETE", "/api/v1/users/viewer/mfa", 403},
		{"admin can reset mfa", "demo", "DELETE", "/api/v1/users/viewer/mfa", 204},
		{"admin can revoke sessions", "demo", "POST", "/api/v1/users/viewer/revoke-sessions", 204},
		{"admin lists directory mfa", "demo", "GET", "/api/v1/users/external-mfa", 204},
		{"operator cannot list directory mfa", "oper", "GET", "/api/v1/users/external-mfa", 403},
		{"viewer cannot list directory mfa", "viewer", "GET", "/api/v1/users/external-mfa", 403},
		{"restricted viewer cannot list directory mfa", "rviewer", "GET", "/api/v1/users/external-mfa", 403},
		{"restricted operator cannot list directory mfa", "roper", "GET", "/api/v1/users/external-mfa", 403},
		{"operator cannot list directory mfa (slash)", "oper", "GET", "/api/v1/users/external-mfa/", 403},
		// --- GitOps application creation: admin-only; list and sync unchanged ---
		{"viewer gitops create", "viewer", "POST", "/api/v1/gitops", 403},
		{"viewer gitops preview", "viewer", "POST", "/api/v1/gitops/preview", 403},
		{"operator gitops create", "oper", "POST", "/api/v1/gitops", 403},
		{"operator gitops preview", "oper", "POST", "/api/v1/gitops/preview", 403},
		{"operator gitops create trailing slash", "oper", "POST", "/api/v1/gitops/", 403},
		{"restricted viewer gitops create", "rviewer", "POST", "/api/v1/gitops", 403},
		{"restricted viewer gitops preview", "rviewer", "POST", "/api/v1/gitops/preview", 403},
		{"restricted operator gitops create", "roper", "POST", "/api/v1/gitops", 403},
		{"restricted operator gitops preview", "roper", "POST", "/api/v1/gitops/preview", 403},
		{"admin gitops create", "demo", "POST", "/api/v1/gitops", 204},
		{"admin gitops preview", "demo", "POST", "/api/v1/gitops/preview", 204},
		{"viewer gitops list ok", "viewer", "GET", "/api/v1/gitops", 204},
		{"operator gitops list ok", "oper", "GET", "/api/v1/gitops", 204},
		{"restricted viewer gitops list", "rviewer", "GET", "/api/v1/gitops", 403},
		{"restricted operator gitops list", "roper", "GET", "/api/v1/gitops", 403},
		{"operator gitops sync still allowed", "oper", "POST", "/api/v1/gitops/Application/argocd/app/sync", 204},
		{"viewer gitops sync", "viewer", "POST", "/api/v1/gitops/Application/argocd/app/sync", 403},
		{"restricted operator gitops sync", "roper", "POST", "/api/v1/gitops/Application/argocd/app/sync", 403},
		{"operator gitops delete", "oper", "DELETE", "/api/v1/gitops/Application/argocd/app", 403},
		{"operator gitops delete trailing slash", "oper", "DELETE", "/api/v1/gitops/Application/argocd/app/", 403},
		{"operator gitops delete sync path", "oper", "DELETE", "/api/v1/gitops/Application/argocd/app/sync", 403},
		{"operator gitops put sync path", "oper", "PUT", "/api/v1/gitops/Application/argocd/app/sync", 403},
		{"operator gitops post nested sync", "oper", "POST", "/api/v1/gitops/Application/argocd/app/x/sync", 403},
		{"viewer gitops delete", "viewer", "DELETE", "/api/v1/gitops/Kustomization/flux-system/apps", 403},
		{"restricted operator gitops delete", "roper", "DELETE", "/api/v1/gitops/HelmRelease/payments/app", 403},
		{"admin gitops delete", "demo", "DELETE", "/api/v1/gitops/Application/argocd/app", 204},
		{"viewer gitops delete preview ok", "viewer", "GET", "/api/v1/gitops/Application/argocd/app/delete-preview", 204},
		{"operator gitops delete preview ok", "oper", "GET", "/api/v1/gitops/Application/argocd/app/delete-preview", 204},
		{"admin gitops delete preview ok", "demo", "GET", "/api/v1/gitops/Application/argocd/app/delete-preview", 204},
		{"restricted viewer gitops delete preview", "rviewer", "GET", "/api/v1/gitops/Application/argocd/app/delete-preview", 403},
		// --- Fleet: cluster-wide, so never for namespace-restricted accounts ---
		{"viewer reads fleet", "viewer", "GET", "/api/v1/fleet", 204},
		{"operator reads fleet", "oper", "GET", "/api/v1/fleet", 204},
		{"restricted viewer fleet", "rviewer", "GET", "/api/v1/fleet", 403},
		{"restricted operator fleet", "roper", "GET", "/api/v1/fleet", 403},
		{"viewer can refresh fleet", "viewer", "POST", "/api/v1/fleet/refresh", 204},
		{"restricted viewer cannot refresh fleet", "rviewer", "POST", "/api/v1/fleet/refresh", 403},
		{"fleet refresh exemption is exact", "viewer", "POST", "/api/v1/fleet/refresh/x", 403},
		{"viewer searches fleet", "viewer", "GET", "/api/v1/fleet/search?q=pay", 204},
		{"operator searches fleet", "oper", "GET", "/api/v1/fleet/search?q=pay", 204},
		{"admin searches fleet", "demo", "GET", "/api/v1/fleet/search?q=pay", 204},
		{"restricted viewer fleet search", "rviewer", "GET", "/api/v1/fleet/search?q=pay", 403},
		{"restricted operator fleet search", "roper", "GET", "/api/v1/fleet/search?q=pay", 403},
		{"fleet prefix does not cover fleetx", "rviewer", "GET", "/api/v1/fleetx", 204},
		{"viewer reads fleet history", "viewer", "GET", "/api/v1/fleet/history?window=1h", 204},
		{"operator reads fleet history", "oper", "GET", "/api/v1/fleet/history?cluster=a", 204},
		{"admin reads fleet history", "demo", "GET", "/api/v1/fleet/history", 204},
		{"restricted viewer fleet history", "rviewer", "GET", "/api/v1/fleet/history", 403},
		{"restricted operator fleet history", "roper", "GET", "/api/v1/fleet/history?window=24h", 403},
		// --- Helm chart catalog: reads for every unrestricted role, repo writes admin-only ---
		{"viewer lists helm repos", "viewer", "GET", "/api/v1/helm/repos", 204},
		{"operator lists helm repos", "oper", "GET", "/api/v1/helm/repos", 204},
		{"admin lists helm repos", "demo", "GET", "/api/v1/helm/repos", 204},
		{"restricted viewer helm repos", "rviewer", "GET", "/api/v1/helm/repos", 403},
		{"restricted operator helm repos", "roper", "GET", "/api/v1/helm/repos", 403},
		{"viewer searches charts", "viewer", "GET", "/api/v1/helm/charts?q=nginx&refresh=true", 204},
		{"operator searches charts", "oper", "GET", "/api/v1/helm/charts", 204},
		{"admin searches charts", "demo", "GET", "/api/v1/helm/charts", 204},
		{"restricted viewer charts", "rviewer", "GET", "/api/v1/helm/charts", 403},
		{"restricted operator charts", "roper", "GET", "/api/v1/helm/charts?repo=x", 403},
		{"viewer chart detail", "viewer", "GET", "/api/v1/helm/charts/bitnami/nginx", 204},
		{"operator chart detail", "oper", "GET", "/api/v1/helm/charts/bitnami/nginx", 204},
		{"admin chart detail", "demo", "GET", "/api/v1/helm/charts/bitnami/nginx", 204},
		{"restricted viewer chart detail", "rviewer", "GET", "/api/v1/helm/charts/bitnami/nginx", 403},
		{"restricted operator chart detail", "roper", "GET", "/api/v1/helm/charts/bitnami/nginx", 403},
		{"viewer chart values", "viewer", "GET", "/api/v1/helm/charts/bitnami/nginx/values?version=1.0.0", 204},
		{"operator chart values", "oper", "GET", "/api/v1/helm/charts/bitnami/nginx/values", 204},
		{"admin chart values", "demo", "GET", "/api/v1/helm/charts/bitnami/nginx/values", 204},
		{"restricted viewer chart values", "rviewer", "GET", "/api/v1/helm/charts/bitnami/nginx/values", 403},
		{"restricted operator chart values", "roper", "GET", "/api/v1/helm/charts/bitnami/nginx/values", 403},
		{"viewer cannot add helm repo", "viewer", "POST", "/api/v1/helm/repos", 403},
		{"operator cannot add helm repo", "oper", "POST", "/api/v1/helm/repos", 403},
		{"restricted viewer cannot add helm repo", "rviewer", "POST", "/api/v1/helm/repos", 403},
		{"restricted operator cannot add helm repo", "roper", "POST", "/api/v1/helm/repos", 403},
		{"admin adds helm repo", "demo", "POST", "/api/v1/helm/repos", 204},
		{"viewer cannot remove helm repo", "viewer", "DELETE", "/api/v1/helm/repos/bitnami", 403},
		{"operator cannot remove helm repo", "oper", "DELETE", "/api/v1/helm/repos/bitnami", 403},
		{"restricted viewer cannot remove helm repo", "rviewer", "DELETE", "/api/v1/helm/repos/bitnami", 403},
		{"restricted operator cannot remove helm repo", "roper", "DELETE", "/api/v1/helm/repos/bitnami", 403},
		{"admin removes helm repo", "demo", "DELETE", "/api/v1/helm/repos/bitnami", 204},
		{"helm repos prefix is not a wildcard", "oper", "POST", "/api/v1/helm/reposx", 204},
		// --- Helm registry credentials: admin-only for every method, GET included ---
		{"viewer cannot list registry creds", "viewer", "GET", "/api/v1/helm/registry-credentials", 403},
		{"operator cannot list registry creds", "oper", "GET", "/api/v1/helm/registry-credentials", 403},
		{"restricted viewer cannot list registry creds", "rviewer", "GET", "/api/v1/helm/registry-credentials", 403},
		{"restricted operator cannot list registry creds", "roper", "GET", "/api/v1/helm/registry-credentials", 403},
		{"admin lists registry creds", "demo", "GET", "/api/v1/helm/registry-credentials", 204},
		{"admin lists registry creds trailing slash", "demo", "GET", "/api/v1/helm/registry-credentials/", 204},
		{"viewer cannot list registry creds trailing slash", "viewer", "GET", "/api/v1/helm/registry-credentials/", 403},
		{"viewer cannot add registry creds", "viewer", "POST", "/api/v1/helm/registry-credentials", 403},
		{"operator cannot add registry creds", "oper", "POST", "/api/v1/helm/registry-credentials", 403},
		{"restricted viewer cannot add registry creds", "rviewer", "POST", "/api/v1/helm/registry-credentials", 403},
		{"restricted operator cannot add registry creds", "roper", "POST", "/api/v1/helm/registry-credentials", 403},
		{"admin adds registry creds", "demo", "POST", "/api/v1/helm/registry-credentials", 204},
		{"viewer cannot delete registry creds", "viewer", "DELETE", "/api/v1/helm/registry-credentials/ghcr", 403},
		{"operator cannot delete registry creds", "oper", "DELETE", "/api/v1/helm/registry-credentials/ghcr", 403},
		{"restricted viewer cannot delete registry creds", "rviewer", "DELETE", "/api/v1/helm/registry-credentials/ghcr", 403},
		{"restricted operator cannot delete registry creds", "roper", "DELETE", "/api/v1/helm/registry-credentials/ghcr", 403},
		{"admin deletes registry creds", "demo", "DELETE", "/api/v1/helm/registry-credentials/ghcr", 204},
		{"registry creds prefix is not a wildcard", "viewer", "GET", "/api/v1/helm/registry-credentialsx", 204},
		// --- Autoscaling views (VPA, KEDA): namespaced reads, like /hpas ---
		{"viewer lists vpas", "viewer", "GET", "/api/v1/vpas", 204},
		{"operator lists vpas", "oper", "GET", "/api/v1/vpas?namespace=all", 204},
		{"admin lists vpas", "demo", "GET", "/api/v1/vpas", 204},
		{"restricted viewer vpas own namespace", "rviewer", "GET", "/api/v1/vpas?namespace=team-a", 204},
		{"restricted viewer vpas other namespace", "rviewer", "GET", "/api/v1/vpas?namespace=team-b", 403},
		{"restricted viewer vpas missing namespace", "rviewer", "GET", "/api/v1/vpas", 403},
		{"restricted viewer vpas all namespaces", "rviewer", "GET", "/api/v1/vpas?namespace=all", 403},
		{"restricted operator vpas other namespace", "roper", "GET", "/api/v1/vpas?namespace=kube-system", 403},
		{"viewer lists scaledobjects", "viewer", "GET", "/api/v1/scaledobjects", 204},
		{"operator lists scaledobjects", "oper", "GET", "/api/v1/scaledobjects", 204},
		{"admin lists scaledobjects", "demo", "GET", "/api/v1/scaledobjects?namespace=all", 204},
		{"restricted viewer scaledobjects own namespace", "rviewer", "GET", "/api/v1/scaledobjects?namespace=team-a", 204},
		{"restricted viewer scaledobjects other namespace", "rviewer", "GET", "/api/v1/scaledobjects?namespace=team-b", 403},
		{"restricted viewer scaledobjects missing namespace", "rviewer", "GET", "/api/v1/scaledobjects", 403},
		{"restricted viewer scaledobjects all namespaces", "rviewer", "GET", "/api/v1/scaledobjects?namespace=all", 403},
		{"restricted operator scaledobjects own namespace", "roper", "GET", "/api/v1/scaledobjects?namespace=team-a", 204},
		// --- VPA / KEDA create, preview, delete: operator+admin; the POST
		// body's namespace is checked by the handler (api/autoscaling_create.go),
		// the DELETE path namespace by the guard ---
		{"viewer cannot create vpa", "viewer", "POST", "/api/v1/vpas", 403},
		{"viewer cannot preview vpa", "viewer", "POST", "/api/v1/vpas/preview", 403},
		{"viewer cannot delete vpa", "viewer", "DELETE", "/api/v1/vpas/team-a/x", 403},
		{"operator creates vpa", "oper", "POST", "/api/v1/vpas", 204},
		{"operator previews vpa", "oper", "POST", "/api/v1/vpas/preview", 204},
		{"operator deletes vpa", "oper", "DELETE", "/api/v1/vpas/kube-system/x", 204},
		{"admin creates vpa", "demo", "POST", "/api/v1/vpas", 204},
		{"admin previews vpa", "demo", "POST", "/api/v1/vpas/preview", 204},
		{"admin deletes vpa", "demo", "DELETE", "/api/v1/vpas/team-b/x", 204},
		{"restricted viewer cannot create vpa", "rviewer", "POST", "/api/v1/vpas", 403},
		{"restricted viewer cannot preview vpa", "rviewer", "POST", "/api/v1/vpas/preview", 403},
		{"restricted viewer cannot delete vpa own namespace", "rviewer", "DELETE", "/api/v1/vpas/team-a/x", 403},
		{"restricted operator creates vpa (body checked by handler)", "roper", "POST", "/api/v1/vpas", 204},
		{"restricted operator previews vpa (body checked by handler)", "roper", "POST", "/api/v1/vpas/preview", 204},
		{"restricted operator deletes vpa own namespace", "roper", "DELETE", "/api/v1/vpas/team-a/x", 204},
		{"restricted operator cannot delete vpa other namespace", "roper", "DELETE", "/api/v1/vpas/team-b/x", 403},
		{"restricted operator vpa delete path wins over query", "roper", "DELETE", "/api/v1/vpas/team-b/x?namespace=team-a", 403},
		{"restricted operator vpa other POST under prefix", "roper", "POST", "/api/v1/vpas/team-b/x", 403},
		{"viewer cannot create scaledobject", "viewer", "POST", "/api/v1/scaledobjects", 403},
		{"viewer cannot preview scaledobject", "viewer", "POST", "/api/v1/scaledobjects/preview", 403},
		{"viewer cannot delete scaledobject", "viewer", "DELETE", "/api/v1/scaledobjects/team-a/x", 403},
		{"operator creates scaledobject", "oper", "POST", "/api/v1/scaledobjects", 204},
		{"operator previews scaledobject", "oper", "POST", "/api/v1/scaledobjects/preview", 204},
		{"operator deletes scaledobject", "oper", "DELETE", "/api/v1/scaledobjects/payments/x", 204},
		{"admin creates scaledobject", "demo", "POST", "/api/v1/scaledobjects", 204},
		{"admin previews scaledobject", "demo", "POST", "/api/v1/scaledobjects/preview", 204},
		{"admin deletes scaledobject", "demo", "DELETE", "/api/v1/scaledobjects/payments/x", 204},
		{"restricted viewer cannot create scaledobject", "rviewer", "POST", "/api/v1/scaledobjects", 403},
		{"restricted viewer cannot preview scaledobject", "rviewer", "POST", "/api/v1/scaledobjects/preview", 403},
		{"restricted viewer cannot delete scaledobject", "rviewer", "DELETE", "/api/v1/scaledobjects/team-a/x", 403},
		{"restricted operator creates scaledobject (body checked by handler)", "roper", "POST", "/api/v1/scaledobjects", 204},
		{"restricted operator previews scaledobject (body checked by handler)", "roper", "POST", "/api/v1/scaledobjects/preview", 204},
		{"restricted operator deletes scaledobject own namespace", "roper", "DELETE", "/api/v1/scaledobjects/team-a/x", 204},
		{"restricted operator cannot delete scaledobject other namespace", "roper", "DELETE", "/api/v1/scaledobjects/team-b/x", 403},
		{"restricted operator scaledobject delete path wins over query", "roper", "DELETE", "/api/v1/scaledobjects/team-b/x?namespace=team-a", 403},
		// --- VPA / ScaledObject edit: GET .../edit is a namespaced read; preview and PUT are operator/admin ---
		{"viewer reads vpas edit", "viewer", "GET", "/api/v1/vpas/team-a/x/edit", 204},
		{"operator reads vpas edit", "oper", "GET", "/api/v1/vpas/team-b/x/edit", 204},
		{"admin reads vpas edit", "demo", "GET", "/api/v1/vpas/team-b/x/edit", 204},
		{"restricted viewer reads vpas edit own namespace", "rviewer", "GET", "/api/v1/vpas/team-a/x/edit", 204},
		{"restricted viewer cannot read vpas edit other namespace", "rviewer", "GET", "/api/v1/vpas/other-ns/x/edit", 403},
		{"restricted operator reads vpas edit own namespace", "roper", "GET", "/api/v1/vpas/team-a/x/edit", 204},
		{"restricted operator cannot read vpas edit other namespace", "roper", "GET", "/api/v1/vpas/other-ns/x/edit", 403},
		{"restricted operator vpas edit path wins over query", "roper", "GET", "/api/v1/vpas/other-ns/x/edit?namespace=team-a", 403},
		{"viewer cannot preview vpas edit", "viewer", "POST", "/api/v1/vpas/team-a/x/edit/preview", 403},
		{"operator previews vpas edit", "oper", "POST", "/api/v1/vpas/team-b/x/edit/preview", 204},
		{"admin previews vpas edit", "demo", "POST", "/api/v1/vpas/team-b/x/edit/preview", 204},
		{"restricted viewer cannot preview vpas edit own namespace", "rviewer", "POST", "/api/v1/vpas/team-a/x/edit/preview", 403},
		{"restricted operator previews vpas edit own namespace", "roper", "POST", "/api/v1/vpas/team-a/x/edit/preview", 204},
		{"restricted operator cannot preview vpas edit other namespace", "roper", "POST", "/api/v1/vpas/other-ns/x/edit/preview", 403},
		{"viewer cannot put vpas", "viewer", "PUT", "/api/v1/vpas/team-a/x", 403},
		{"operator puts vpas", "oper", "PUT", "/api/v1/vpas/team-b/x", 204},
		{"admin puts vpas", "demo", "PUT", "/api/v1/vpas/team-b/x", 204},
		{"restricted viewer cannot put vpas own namespace", "rviewer", "PUT", "/api/v1/vpas/team-a/x", 403},
		{"restricted operator puts vpas own namespace", "roper", "PUT", "/api/v1/vpas/team-a/x", 204},
		{"restricted operator cannot put vpas other namespace", "roper", "PUT", "/api/v1/vpas/other-ns/x", 403},
		{"restricted operator vpas put path wins over query", "roper", "PUT", "/api/v1/vpas/other-ns/x?namespace=team-a", 403},
		{"viewer reads scaledobjects edit", "viewer", "GET", "/api/v1/scaledobjects/team-a/x/edit", 204},
		{"operator reads scaledobjects edit", "oper", "GET", "/api/v1/scaledobjects/team-b/x/edit", 204},
		{"admin reads scaledobjects edit", "demo", "GET", "/api/v1/scaledobjects/team-b/x/edit", 204},
		{"restricted viewer reads scaledobjects edit own namespace", "rviewer", "GET", "/api/v1/scaledobjects/team-a/x/edit", 204},
		{"restricted viewer cannot read scaledobjects edit other namespace", "rviewer", "GET", "/api/v1/scaledobjects/other-ns/x/edit", 403},
		{"restricted operator reads scaledobjects edit own namespace", "roper", "GET", "/api/v1/scaledobjects/team-a/x/edit", 204},
		{"restricted operator cannot read scaledobjects edit other namespace", "roper", "GET", "/api/v1/scaledobjects/other-ns/x/edit", 403},
		{"restricted operator scaledobjects edit path wins over query", "roper", "GET", "/api/v1/scaledobjects/other-ns/x/edit?namespace=team-a", 403},
		{"viewer cannot preview scaledobjects edit", "viewer", "POST", "/api/v1/scaledobjects/team-a/x/edit/preview", 403},
		{"operator previews scaledobjects edit", "oper", "POST", "/api/v1/scaledobjects/team-b/x/edit/preview", 204},
		{"admin previews scaledobjects edit", "demo", "POST", "/api/v1/scaledobjects/team-b/x/edit/preview", 204},
		{"restricted viewer cannot preview scaledobjects edit own namespace", "rviewer", "POST", "/api/v1/scaledobjects/team-a/x/edit/preview", 403},
		{"restricted operator previews scaledobjects edit own namespace", "roper", "POST", "/api/v1/scaledobjects/team-a/x/edit/preview", 204},
		{"restricted operator cannot preview scaledobjects edit other namespace", "roper", "POST", "/api/v1/scaledobjects/other-ns/x/edit/preview", 403},
		{"viewer cannot put scaledobjects", "viewer", "PUT", "/api/v1/scaledobjects/team-a/x", 403},
		{"operator puts scaledobjects", "oper", "PUT", "/api/v1/scaledobjects/team-b/x", 204},
		{"admin puts scaledobjects", "demo", "PUT", "/api/v1/scaledobjects/team-b/x", 204},
		{"restricted viewer cannot put scaledobjects own namespace", "rviewer", "PUT", "/api/v1/scaledobjects/team-a/x", 403},
		{"restricted operator puts scaledobjects own namespace", "roper", "PUT", "/api/v1/scaledobjects/team-a/x", 204},
		{"restricted operator cannot put scaledobjects other namespace", "roper", "PUT", "/api/v1/scaledobjects/other-ns/x", 403},
		{"restricted operator scaledobjects put path wins over query", "roper", "PUT", "/api/v1/scaledobjects/other-ns/x?namespace=team-a", 403},
		// --- TriggerAuthentication names: a namespaced read ---
		{"viewer lists triggerauthentications", "viewer", "GET", "/api/v1/triggerauthentications", 204},
		{"operator lists triggerauthentications", "oper", "GET", "/api/v1/triggerauthentications?namespace=payments", 204},
		{"admin lists triggerauthentications", "demo", "GET", "/api/v1/triggerauthentications", 204},
		{"restricted viewer triggerauthentications own namespace", "rviewer", "GET", "/api/v1/triggerauthentications?namespace=team-a", 204},
		{"restricted viewer triggerauthentications other namespace", "rviewer", "GET", "/api/v1/triggerauthentications?namespace=team-b", 403},
		{"restricted viewer triggerauthentications missing namespace", "rviewer", "GET", "/api/v1/triggerauthentications", 403},
		{"restricted operator triggerauthentications own namespace", "roper", "GET", "/api/v1/triggerauthentications?namespace=team-a", 204},
		{"restricted operator triggerauthentications other namespace", "roper", "GET", "/api/v1/triggerauthentications?namespace=team-b", 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, nil)
			pw := "pw"
			if tc.user == "demo" {
				pw = "demo"
			}
			r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(tc.user+":"+pw)))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("%s %s as %s: got %d, want %d", tc.method, tc.path, tc.user, w.Code, tc.want)
			}
		})
	}
}

// Require-MFA policy rows: under AXIOM_REQUIRE_MFA=operator, unenrolled
// operators and admins reach only enrolment and sign-out; viewers, the
// break-glass env admin ("demo", no AXIOM_TOTP_SECRET) and enrolled accounts
// are unaffected.
func TestGuardPolicyRequireMFA(t *testing.T) {
	users := newPolicyUsers()
	g := Guard{Username: "demo", Password: "demo", Users: users, MFA: users, RequireMFA: MFAPolicyOperator}
	h := g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	cases := []struct {
		name, user, method, path string
		want                     int
	}{
		{"unenrolled operator overview", "op", "GET", "/api/v1/overview", 403},
		{"unenrolled operator apply", "op", "POST", "/api/v1/apply", 403},
		{"unenrolled admin users", "adm", "GET", "/api/v1/users", 403},
		{"unenrolled operator session", "op", "GET", "/api/v1/session", 204},
		{"unenrolled operator mfa status", "op", "GET", "/api/v1/auth/mfa", 204},
		{"unenrolled operator mfa setup", "op", "POST", "/api/v1/auth/mfa/setup", 204},
		{"unenrolled operator mfa enable", "op", "POST", "/api/v1/auth/mfa/enable", 204},
		{"unenrolled operator mfa disable", "op", "POST", "/api/v1/auth/mfa/disable", 204},
		{"unenrolled operator logout-all", "op", "POST", "/api/v1/auth/logout-all", 204},
		{"unenrolled admin logout", "adm", "POST", "/api/v1/auth/logout", 204},
		{"unenrolled admin providers", "adm", "GET", "/api/v1/auth/providers", 204},
		{"unenrolled operator ticket", "op", "POST", "/api/v1/ws-ticket", 403},
		{"uncovered viewer overview", "view", "GET", "/api/v1/overview", 204},
		{"uncovered restricted viewer own ns", "rview", "GET", "/api/v1/pods?namespace=team-a", 204},
		{"break-glass env admin", "demo", "GET", "/api/v1/users", 204},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, nil)
			pw := "pw"
			if tc.user == "demo" {
				pw = "demo"
			}
			r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(tc.user+":"+pw)))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("%s %s as %s: got %d, want %d (%s)", tc.method, tc.path, tc.user, w.Code, tc.want, w.Body.String())
			}
		})
	}
}
