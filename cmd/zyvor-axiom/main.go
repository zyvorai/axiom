package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/zyvorai/axiom/internal/api"
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

	mux := http.NewServeMux()
	mux.Handle("/api/", api.NewWithUsers(provider, cfg.Mode, cfg.Username, cfg.Password, cfg.AllowOrigin, hub, users, api.AuthOptions{
		APIKey:      cfg.APIKey,
		OIDCLogin:   firstNonEmpty(cur.OIDCLoginURL, cfg.OIDCLogin),
		OIDCIssuer:  firstNonEmpty(cur.OIDCIssuer, cfg.OIDCIssuer),
		OIDCClient:  firstNonEmpty(cur.OIDCClientID, cfg.OIDCClient),
		SAMLLogin:   cfg.SAMLLogin,
		LDAPEnabled: cfg.LDAPEnabled,
		LDAPDomain:  cfg.LDAPDomain,
		LDAPURL:     cfg.LDAPURL,
	}))
	mux.Handle("/", ui.Handler())
	srv := &http.Server{Addr: cfg.Addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	tlsEnabled := cfg.TLSCert != "" && cfg.TLSKey != ""
	go func() {
		log.Printf("Zyvor Axiom listening on %s mode=%s tls=%v", cfg.Addr, cfg.Mode, tlsEnabled)
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
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
