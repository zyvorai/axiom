package api

import (
	"net/http"

	"github.com/zyvorai/axiom/internal/platform"
)

func (s *Server) getIntegrations(w http.ResponseWriter, r *http.Request) {
	if s.hub == nil {
		writeJSON(w, http.StatusOK, platform.Integrations{}.View())
		return
	}
	writeJSON(w, http.StatusOK, s.hub.View())
}

func (s *Server) getIntegrationExamples(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"examples": platform.IntegrationExamples()})
}

func (s *Server) putIntegrations(w http.ResponseWriter, r *http.Request) {
	if s.hub == nil {
		fail(w, http.StatusServiceUnavailable, errSSO("integrations hub not available"))
		return
	}
	var patch platform.IntegrationsPatch
	if err := decode(r, &patch); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	if err := platform.ValidateAIPatch(patch); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	if err := platform.ValidateBillingPatch(patch); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	view, err := s.hub.Update(r.Context(), patch)
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	cur := s.hub.Get()
	s.auth.OIDCIssuer = cur.OIDCIssuer
	s.auth.OIDCLogin = cur.OIDCLoginURL
	s.auth.OIDCClient = cur.OIDCClientID
	platform.RecordAxiomAudit("integrations.update", "settings", "", s.auditUser(r))
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) writeConnectionTest(w http.ResponseWriter, res platform.ConnectionTestResult) {
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) testAtlas(w http.ResponseWriter, r *http.Request) {
	s.writeConnectionTest(w, platform.TestAtlasConnection(r.Context(), s.atlasClient()))
}

func (s *Server) testOIDC(w http.ResponseWriter, r *http.Request) {
	issuer := ""
	if s.hub != nil {
		issuer = s.hub.Get().OIDCIssuer
	}
	if issuer == "" {
		issuer = s.auth.OIDCIssuer
	}
	s.writeConnectionTest(w, platform.TestOIDCConnection(r.Context(), issuer))
}

func (s *Server) testPostgres(w http.ResponseWriter, r *http.Request) {
	if s.hub == nil {
		s.writeConnectionTest(w, platform.ConnectionTestResult{Service: "postgres", Error: "integrations not available"})
		return
	}
	s.writeConnectionTest(w, platform.TestPostgresConnection(r.Context(), s.hub.Get()))
}

func (s *Server) testProbe(w http.ResponseWriter, r *http.Request) {
	if s.hub == nil {
		s.writeConnectionTest(w, platform.ConnectionTestResult{Service: "probe", Error: "integrations not available"})
		return
	}
	in := s.hub.Get()
	// Optional one-shot overrides (Settings matrix / ad-hoc) without saving.
	var body struct {
		ProbeURL          string `json:"probeUrl"`
		ProbePodNamespace string `json:"probePodNamespace"`
		ProbePodName      string `json:"probePodName"`
		ProbePodContainer string `json:"probePodContainer"`
	}
	if r.Body != nil {
		_ = decode(r, &body)
		if body.ProbeURL != "" {
			in.ProbeURL = body.ProbeURL
		}
		if body.ProbePodNamespace != "" || body.ProbePodName != "" {
			in.ProbePodNamespace = body.ProbePodNamespace
			in.ProbePodName = body.ProbePodName
			in.ProbePodContainer = body.ProbePodContainer
		}
	}
	s.writeConnectionTest(w, platform.TestPodProbe(r.Context(), s.provider, in))
}

func (s *Server) probeMatrix(w http.ResponseWriter, r *http.Request) {
	if s.hub == nil {
		fail(w, 503, errSSO("integrations not available"))
		return
	}
	var req platform.ProbeMatrixRequest
	if err := decode(r, &req); err != nil {
		fail(w, 400, err)
		return
	}
	writeJSON(w, 200, platform.RunProbeMatrix(r.Context(), s.provider, s.hub.Get(), req))
}

func (s *Server) testSlackNotify(w http.ResponseWriter, r *http.Request) {
	if s.hub == nil {
		s.writeConnectionTest(w, platform.ConnectionTestResult{Service: "slack", Error: "integrations not available"})
		return
	}
	res := platform.SendSlackMessage(r.Context(), s.hub.Get().SlackWebhookURL, "Axiom test notification — Slack webhook is wired up correctly.")
	s.writeConnectionTest(w, res)
}

func (s *Server) testAlertWebhook(w http.ResponseWriter, r *http.Request) {
	if s.hub == nil {
		s.writeConnectionTest(w, platform.ConnectionTestResult{Service: "webhook", Error: "integrations not available"})
		return
	}
	res := platform.SendAlertWebhook(r.Context(), s.hub.Get().AlertWebhookURL, map[string]any{
		"source":  "zyvor-axiom",
		"message": "Axiom test notification — webhook is wired up correctly.",
	})
	s.writeConnectionTest(w, res)
}
