package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/zyvorai/axiom/internal/api"
	"github.com/zyvorai/axiom/internal/auth"
	"github.com/zyvorai/axiom/internal/config"
	"github.com/zyvorai/axiom/internal/platform"
	"github.com/zyvorai/axiom/internal/ui"
)

func main() {
	cfg := config.FromEnv()
	provider, err := platform.NewProvider(cfg.Mode, cfg.Kubeconfig, cfg.ClusterName)
	if err != nil {
		log.Fatal(err)
	}
	defaults := platform.Integrations{
		AtlasURL:        cfg.AtlasURL,
		AtlasToken:      cfg.AtlasToken,
		AtlasConsoleURL: strings.TrimSpace(os.Getenv("AXIOM_ATLAS_CONSOLE_URL")),
		OIDCIssuer:      cfg.OIDCIssuer,
		OIDCLoginURL:    cfg.OIDCLogin,
		OIDCClientID:    cfg.OIDCClient,
	}
	hub := platform.NewIntegrationHub(defaults, platform.OpenIntegrationStore(cfg.Mode, provider))
	if err := hub.Load(context.Background()); err != nil {
		log.Printf("integrations load: %v", err)
	}
	if hub.Atlas() != nil && hub.Atlas().Enabled() {
		log.Printf("Atlas storage client enabled url=%s", hub.Get().AtlasURL)
	}
	cur := hub.Get()
	dispatchCtx, stopDispatch := context.WithCancel(context.Background())
	defer stopDispatch()
	go platform.RunAlertDispatcher(dispatchCtx, provider, hub)

	users := platform.NewUserHub(platform.OpenUserStore(cfg.Mode, provider))
	if err := users.Load(context.Background()); err != nil {
		log.Printf("users load: %v", err)
	}
	if n := users.Count(); n > 0 {
		log.Printf("local user accounts loaded: %d", n)
	}

	platform.SetAuditRingSize(cfg.AuditRingSize)
	platform.SetAuditStore(platform.OpenAuditStore(cfg.Mode, provider))
	platform.SetNotifyLeader(platform.OpenNotifyLeader(cfg.Mode, provider))
	platform.SetRemoteClusterStore(platform.OpenRemoteClusterStore(cfg.Mode, provider))
	notifier := platform.NewHubNotifier(hub)
	var fleetHistoryFlushed <-chan struct{}
	if sp, ok := provider.(*platform.SwitchableProvider); ok {
		platform.LoadRemoteClustersInto(context.Background(), sp)
		sp.SetFleetNotifier(notifier)
		// Restore the persisted fleet history before the first probe; the
		// last save happens when dispatchCtx is cancelled at shutdown.
		fleetHistoryFlushed = sp.StartFleetHistoryPersistence(dispatchCtx, platform.OpenFleetHistoryStore(cfg.Mode, provider), cfg.FleetHistoryPersistInterval)
		sp.StartFleetProber(dispatchCtx, cfg.FleetProbeInterval)
	}
	// One budget store shared by the API and the budget watcher (the demo
	// store is in memory, so two instances would not see each other's writes).
	costBudgets := platform.OpenCostBudgetStore(cfg.Mode, provider)
	platform.StartBudgetWatcher(dispatchCtx, cfg.BudgetCheckInterval, provider, costBudgets, notifier)
	autoAgents := &platform.AgentAutoRunner{Provider: provider, Hub: hub, Demo: cfg.Mode == "demo"}
	autoAgents.Start()
	defer autoAgents.Stop()

	auth.SetSessionTTL(cfg.SessionTTL)

	if prom, err := platform.NewPrometheusClient(cfg.PrometheusURL, cfg.PrometheusToken); err != nil {
		log.Printf("rightsizing: Prometheus disabled: %v", err)
	} else if prom != nil && platform.ConfigurePrometheus(provider, prom) {
		log.Printf("rightsizing: Prometheus usage history enabled")
	}
	if am, err := platform.NewAlertmanagerClient(cfg.AlertmanagerURL, cfg.AlertmanagerToken); err != nil {
		log.Printf("alertmanager: disabled: %v", err)
	} else if am != nil {
		platform.ConfigureAlertmanager(am)
		log.Printf("alertmanager: silences and firing alerts enabled")
	}
	if oc, err := platform.NewOpenCostClient(cfg.OpenCostURL, cfg.OpenCostToken); err != nil {
		log.Printf("cost: OpenCost disabled: %v", err)
	} else if oc != nil && platform.ConfigureOpenCost(provider, oc) {
		log.Printf("cost: actual allocation cost from OpenCost enabled")
	}

	// Session revocations live in a shared store; re-read it periodically so
	// a logout on another replica applies here within the refresh interval.
	revocations := auth.NewRevocations(platform.OpenRevocationStore(cfg.Mode, provider))
	revocations.StartRefresh(dispatchCtx, cfg.RevocationRefresh)

	// Optional separate origin for pod windows (see internal/api/podproxy_origin.go).
	// With a stable session secret its tickets and sessions are signed tokens
	// every replica accepts, and redeemed tickets are shared through a
	// ConfigMap; without one they stay in this process's memory.
	ppCfg := api.PodProxyOriginConfig{
		Addr: cfg.PodProxyAddr, Origin: cfg.PodProxyOrigin,
		ConsoleAddr: cfg.Addr, ConsoleOrigin: cfg.AllowOrigin,
		SessionSecret: cfg.SessionSecret,
	}
	if cfg.SessionSecret != "" && cfg.PodProxyAddr != "" {
		ppCfg.RedeemedStore = platform.OpenPodProxyRedeemedStore(cfg.Mode, provider)
	}
	podProxy := api.NewPodProxyOriginFromEnv(ppCfg)

	mux := http.NewServeMux()
	mux.Handle("/api/", api.NewWithUsers(provider, cfg.Mode, cfg.Username, cfg.Password, cfg.AllowOrigin, hub, users, api.AuthOptions{
		APIKey:             cfg.APIKey,
		SessionSecret:      cfg.SessionSecret,
		Revocations:        revocations,
		RequireMFA:         cfg.RequireMFA,
		HelmRepoStore:      platform.OpenHelmRepoStore(cfg.Mode, provider),
		CostBudgetStore:    costBudgets,
		ExternalMFAStore:   platform.OpenExternalMFAStore(cfg.Mode, provider),
		TOTPSecret:         cfg.TOTPSecret,
		TrustedProxyHeader: cfg.TrustedProxyHeader,
		OIDCLogin:          firstNonEmpty(cur.OIDCLoginURL, cfg.OIDCLogin),
		OIDCIssuer:         firstNonEmpty(cur.OIDCIssuer, cfg.OIDCIssuer),
		OIDCClient:         firstNonEmpty(cur.OIDCClientID, cfg.OIDCClient),
		SAMLLogin:          cfg.SAMLLogin,
		SAMLEntityID:       cfg.SAMLEntityID,
		SAMLIDPCert:        cfg.SAMLIDPCert,
		SAMLAllowUnsigned:  cfg.SAMLAllowUnsigned,
		LDAPEnabled:        cfg.LDAPEnabled,
		LDAPDomain:         cfg.LDAPDomain,
		LDAPURL:            cfg.LDAPURL,
		LDAPBaseDN:         cfg.LDAPBaseDN,
		LDAPBindDN:         cfg.LDAPBindDN,
		LDAPBindPW:         cfg.LDAPBindPW,
		LDAPUserAttr:       cfg.LDAPUserAttr,
		LDAPGroupAttr:      cfg.LDAPGroupAttr,
		LDAPAdminGroup:     cfg.LDAPAdminGroup,
		LDAPViewerGroup:    cfg.LDAPViewerGroup,

		OIDCGroupsClaim: cfg.OIDCGroupsClaim,
		OIDCAdminGroup:  cfg.OIDCAdminGroup,
		OIDCViewerGroup: cfg.OIDCViewerGroup,
		OIDCDefaultRole: cfg.OIDCDefaultRole,

		SAMLAllowIdPInitiated: cfg.SAMLAllowIdPInitiated,

		HelmRegistryCredStore: platform.OpenHelmRegistryCredStore(cfg.Mode, provider),

		PodProxy:          podProxy,
		SAMLRequestStore:  platform.OpenSAMLRequestStore(cfg.Mode, provider),
		StreamTicketStore: platform.OpenStreamTicketStore(cfg.Mode, provider),
	}))
	mux.Handle("/", ui.Handler())
	// Disable HTTP/2: gorilla/websocket upgrades are HTTP/1.1 only, and some
	// browsers mishandle wss against an h2-advertising self-signed endpoint.
	tlsCfg, err := buildServerTLS(cfg)
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
		TLSNextProto:      map[string]func(*http.Server, *tls.Conn, http.Handler){},
		TLSConfig:         tlsCfg,
	}
	if cfg.Mode != "demo" && cfg.Username == "demo" && cfg.Password == "demo" {
		log.Printf("WARNING: running in %q mode with the default demo/demo credentials — set AXIOM_USERNAME/AXIOM_PASSWORD (Helm: auth.username/auth.password) to something strong before exposing this console", cfg.Mode)
	}
	tlsEnabled := cfg.TLSCert != "" && cfg.TLSKey != ""
	go func() {
		log.Printf("Zyvor Axiom listening on %s mode=%s tls=%v mtls=%s", cfg.Addr, cfg.Mode, tlsEnabled, mtlsModeLabel(cfg, tlsCfg))
		var err error
		if tlsEnabled {
			err = srv.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey)
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	if podProxy != nil {
		// Stopped with the process: dispatchCtx is cancelled when main returns.
		ppCert, ppKey := api.PodProxyTLSFiles(cfg.PodProxyTLSCert, cfg.PodProxyTLSKey, cfg.TLSCert, cfg.TLSKey)
		if _, err := api.StartPodProxyOrigin(dispatchCtx, podProxy, ppCert, ppKey); err != nil {
			log.Fatalf("pod proxy origin: %v", err)
		}
		log.Printf("pod proxy origin listening on %s origin=%s mode=%s tls=%v shared=%v", podProxy.Addr(), podProxy.Origin(), podProxy.Mode(), ppCert != "" && ppKey != "", podProxy.Shared())
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	stopDispatch()
	if fleetHistoryFlushed != nil {
		select {
		case <-fleetHistoryFlushed:
		case <-ctx.Done():
		}
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// buildServerTLS returns the listener TLS config. When AXIOM_TLS_CLIENT_CA is
// set, mutual TLS is enabled (default require; AXIOM_TLS_MTLS=request|off).
func buildServerTLS(cfg config.Config) (*tls.Config, error) {
	tc := &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.TLSClientCA == "" {
		return tc, nil
	}
	pem, err := os.ReadFile(cfg.TLSClientCA)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("AXIOM_TLS_CLIENT_CA did not contain any certificates")
	}
	tc.ClientCAs = pool
	mode := cfg.TLSMTLS
	if mode == "" {
		mode = "require"
	}
	switch mode {
	case "require":
		tc.ClientAuth = tls.RequireAndVerifyClientCert
	case "request":
		tc.ClientAuth = tls.VerifyClientCertIfGiven
	case "off":
		tc.ClientAuth = tls.NoClientCert
	default:
		return nil, errors.New(`AXIOM_TLS_MTLS must be "require", "request", or "off"`)
	}
	return tc, nil
}

func mtlsModeLabel(cfg config.Config, tc *tls.Config) string {
	if cfg.TLSClientCA == "" || tc == nil {
		return "off"
	}
	switch tc.ClientAuth {
	case tls.RequireAndVerifyClientCert:
		return "require"
	case tls.VerifyClientCertIfGiven:
		return "request"
	default:
		return "off"
	}
}
