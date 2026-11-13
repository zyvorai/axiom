package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/zyvorai/axiom/internal/platform"
)

func TestHelmHistoryDiffAndRollbackEndpoints(t *testing.T) {
	e := newGitOpsEnv(t)
	base := "/api/v1/helm/releases/monitoring/prometheus"

	var revs []platform.HelmRevision
	w := e.do("GET", base+"/revisions", "", asAdmin())
	if w.Code != http.StatusOK {
		t.Fatalf("revisions: %d %s", w.Code, w.Body.String())
	}
	decodeInto(t, w, &revs)
	if len(revs) < 2 || revs[0].Revision <= revs[1].Revision || revs[0].ChartVersion == "" || revs[0].Description == "" {
		t.Fatalf("revisions: %+v", revs)
	}

	w = e.do("GET", base+"/diff?from=1&to=2", "", asAdmin())
	var d platform.HelmRevisionDiff
	decodeInto(t, w, &d)
	if w.Code != http.StatusOK || !d.Changed || strings.Contains(w.Body.String(), `"x"`) {
		t.Fatalf("diff: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("GET", base+"/diff?from=1", "", asAdmin()); w.Code != http.StatusBadRequest {
		t.Fatalf("diff missing to: %d", w.Code)
	}
	if w := e.do("GET", base+"/diff?from=1&to=99", "", asAdmin()); w.Code != http.StatusNotFound {
		t.Fatalf("diff unknown revision: %d", w.Code)
	}

	w = e.do("GET", base+"/rollback-preview?revision=1", "", asAdmin())
	var p platform.HelmRollbackPreview
	decodeInto(t, w, &p)
	if w.Code != http.StatusOK || p.Target != 1 || p.ConfirmName != "prometheus" || len(p.Warnings) == 0 {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}

	for _, body := range []string{`{"revision":1}`, `{"revision":1,"confirm":""}`, `{"revision":1,"confirm":"Prometheus"}`} {
		if w := e.do("POST", base+"/rollback", body, asAdmin()); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "confirm=prometheus") {
			t.Fatalf("body %s: %d %s", body, w.Code, w.Body.String())
		}
	}
	if w := e.do("POST", base+"/rollback", `{"revision":1,"confirm":"prometheus"}`, asAdmin()); w.Code != http.StatusAccepted {
		t.Fatalf("rollback: %d %s", w.Code, w.Body.String())
	}
}
