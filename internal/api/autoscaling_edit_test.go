package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/zyvorai/axiom/internal/platform"
)

func editBodyJSON(t *testing.T, v any, mut func(map[string]any)) string {
	t.Helper()
	b, _ := json.Marshal(v)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"editable", "reasons", "readOnly"} {
		delete(m, k)
	}
	if mut != nil {
		mut(m)
	}
	out, _ := json.Marshal(m)
	return string(out)
}

func TestScaledObjectEditEndpoints(t *testing.T) {
	e := newAutoscalerCreateEnv(t, platform.NewDemoProvider())
	oper := withBasic("oper1", "oper-password")
	if w := e.do("POST", "/api/v1/scaledobjects", soBodyJSON, oper); w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	t.Cleanup(func() { e.do("DELETE", "/api/v1/scaledobjects/payments/payments-api-scaler", "", asAdmin()) })

	w := e.do("GET", "/api/v1/scaledobjects/payments/payments-api-scaler/edit", "", withBasic("viewer1", "viewer-password"))
	if w.Code != 200 {
		t.Fatalf("get edit: %d %s", w.Code, w.Body.String())
	}
	for _, k := range []string{`"targetKind":"Deployment"`, `"minReplicas":0`, `"maxReplicas":20`, `"pollingInterval":30`, `"cooldownPeriod":300`,
		`"resourceVersion":"1"`, `"editable":true`, `"reasons":[]`, `"readOnly":["namespace","name","targetKind","targetName"]`, `"authenticationRef":"prom-auth"`} {
		if !strings.Contains(w.Body.String(), k) {
			t.Fatalf("get edit lacks %s: %s", k, w.Body.String())
		}
	}
	var ed platform.ScaledObjectEditable
	decodeInto(t, w, &ed)

	body := editBodyJSON(t, ed, func(m map[string]any) { m["maxReplicas"] = 30 })
	w = e.do("POST", "/api/v1/scaledobjects/payments/payments-api-scaler/edit/preview", body, oper)
	if w.Code != 200 || !strings.Contains(w.Body.String(),
		`"changes":[{"object":"ScaledObject/payments/payments-api-scaler","field":"spec.maxReplicaCount","from":"20","to":"30"}]`) ||
		!strings.Contains(w.Body.String(), `"noop":false`) {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}
	w = e.do("PUT", "/api/v1/scaledobjects/payments/payments-api-scaler", body, oper)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":true`) || !strings.Contains(w.Body.String(), `"changed":["spec.maxReplicaCount"]`) {
		t.Fatalf("put: %d %s", w.Code, w.Body.String())
	}
	found := false
	for _, a := range platform.ListAxiomAudit() {
		if a.Action == "autoscale.keda_edit" && a.Target == "payments/payments-api-scaler" {
			found = true
			if a.Detail != "fields=spec.maxReplicaCount" {
				t.Errorf("audit detail: %q", a.Detail)
			}
		}
	}
	if !found {
		t.Errorf("no keda_edit audit")
	}
	// Same body again: stale resourceVersion.
	if w := e.do("PUT", "/api/v1/scaledobjects/payments/payments-api-scaler", body, oper); w.Code != http.StatusConflict ||
		!strings.Contains(w.Body.String(), "the object changed since you opened it") {
		t.Fatalf("stale: %d %s", w.Code, w.Body.String())
	}
	decodeInto(t, e.do("GET", "/api/v1/scaledobjects/payments/payments-api-scaler/edit", "", oper), &ed)
	cases := []struct {
		name string
		mut  func(map[string]any)
		want int
		text string
	}{
		{"immutable targetName", func(m map[string]any) { m["targetName"] = "other" }, 400, "targetName cannot be changed"},
		{"body namespace differs", func(m map[string]any) { m["namespace"] = "data" }, 400, "does not match"},
		{"invalid", func(m map[string]any) { m["maxReplicas"] = -1 }, 400, "maxReplicas"},
		{"unknown field", func(m map[string]any) { m["yaml"] = "kind: Pod" }, 400, "unknown field"},
		{"unknown auth", func(m map[string]any) {
			m["triggers"].([]any)[1].(map[string]any)["authenticationRef"] = "nope-auth"
		}, 400, "TriggerAuthentication nope-auth not found"},
	}
	for _, c := range cases {
		w := e.do("PUT", "/api/v1/scaledobjects/payments/payments-api-scaler", editBodyJSON(t, ed, c.mut), oper)
		if w.Code != c.want || !strings.Contains(w.Body.String(), c.text) {
			t.Errorf("%s: %d %s", c.name, w.Code, w.Body.String())
		}
	}
	if w := e.do("GET", "/api/v1/scaledobjects/payments/nope/edit", "", oper); w.Code != 404 {
		t.Fatalf("404: %d", w.Code)
	}
}

func TestVPAEditEndpoints(t *testing.T) {
	e := newAutoscalerCreateEnv(t, platform.NewDemoProvider())
	oper := withBasic("oper1", "oper-password")
	if w := e.do("POST", "/api/v1/vpas", vpaBodyJSON, oper); w.Code != http.StatusCreated {
		t.Fatalf("create: %d", w.Code)
	}
	t.Cleanup(func() { e.do("DELETE", "/api/v1/vpas/commerce/checkout-rec", "", asAdmin()) })
	var ed platform.VPAEditable
	w := e.do("GET", "/api/v1/vpas/commerce/checkout-rec/edit", "", oper)
	decodeInto(t, w, &ed)
	if w.Code != 200 || !ed.Editable || len(ed.Containers) != 1 || ed.UpdateMode != "Off" {
		t.Fatalf("get: %d %s", w.Code, w.Body.String())
	}
	body := editBodyJSON(t, ed, func(m map[string]any) { m["updateMode"] = "Recreate"; m["containers"] = []any{} })
	w = e.do("POST", "/api/v1/vpas/commerce/checkout-rec/edit/preview", body, oper)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "spec.resourcePolicy.containerPolicies[app]") || !strings.Contains(w.Body.String(), "pods may be restarted") {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}
	w = e.do("PUT", "/api/v1/vpas/commerce/checkout-rec", body, oper)
	// Recreate warning plus the demo checkout HPA conflict warning.
	if w.Code != 200 || !strings.Contains(w.Body.String(), "VPA and HPA on the same resource conflict") ||
		!strings.Contains(w.Body.String(), `"changed":["spec.updatePolicy.updateMode","spec.resourcePolicy.containerPolicies[app]"]`) {
		t.Fatalf("put: %d %s", w.Code, w.Body.String())
	}
	if !hasAudit("autoscale.vpa_edit", "commerce/checkout-rec") {
		t.Errorf("no vpa_edit audit")
	}
}

func TestAutoscalerEditRoles(t *testing.T) {
	e := newAutoscalerCreateEnv(t, platform.NewDemoProvider())
	viewer := withBasic("viewer1", "viewer-password")
	oper := withBasic("oper1", "oper-password")
	scoped := withBasic("scoped", "scoped-password")
	scopedop := withBasic("scopedop", "scopedop-password")

	// Bodies with the current resourceVersion; a no-op PUT is a 200.
	var so platform.ScaledObjectEditable
	decodeInto(t, e.do("GET", "/api/v1/scaledobjects/payments/payments-api/edit", "", asAdmin()), &so)
	soBody := editBodyJSON(t, so, nil)
	var vpa platform.VPAEditable
	decodeInto(t, e.do("GET", "/api/v1/vpas/data/redis-vpa/edit", "", asAdmin()), &vpa)
	vpaBody := editBodyJSON(t, vpa, nil)
	dataSO := `{"resourceVersion":"1"}`

	cases := []struct {
		name, method, path, body string
		as                       reqOpt
		want                     int
	}{
		{"viewer get so edit", "GET", "/api/v1/scaledobjects/payments/payments-api/edit", "", viewer, 200},
		{"viewer get vpa edit", "GET", "/api/v1/vpas/data/redis-vpa/edit", "", viewer, 200},
		{"viewer preview so edit", "POST", "/api/v1/scaledobjects/payments/payments-api/edit/preview", soBody, viewer, 403},
		{"viewer put so", "PUT", "/api/v1/scaledobjects/payments/payments-api", soBody, viewer, 403},
		{"viewer put vpa", "PUT", "/api/v1/vpas/data/redis-vpa", vpaBody, viewer, 403},
		{"operator preview vpa edit", "POST", "/api/v1/vpas/data/redis-vpa/edit/preview", vpaBody, oper, 200},
		{"operator put vpa", "PUT", "/api/v1/vpas/data/redis-vpa", vpaBody, oper, 200},
		{"admin put so", "PUT", "/api/v1/scaledobjects/payments/payments-api", soBody, asAdmin(), 200},
		{"restricted viewer get own ns", "GET", "/api/v1/scaledobjects/payments/payments-api/edit", "", scoped, 200},
		{"restricted viewer get other ns", "GET", "/api/v1/vpas/data/redis-vpa/edit", "", scoped, 403},
		{"restricted viewer put own ns", "PUT", "/api/v1/scaledobjects/payments/payments-api", soBody, scoped, 403},
		{"restricted operator get other ns", "GET", "/api/v1/scaledobjects/data/redis-consumer/edit", "", scopedop, 403},
		{"restricted operator preview other ns", "POST", "/api/v1/scaledobjects/data/redis-consumer/edit/preview", dataSO, scopedop, 403},
		{"restricted operator put other ns", "PUT", "/api/v1/vpas/data/redis-vpa", vpaBody, scopedop, 403},
		{"restricted operator put other ns via query", "PUT", "/api/v1/vpas/data/redis-vpa?namespace=payments", vpaBody, scopedop, 403},
		{"restricted operator body ns differs", "PUT", "/api/v1/scaledobjects/payments/payments-api",
			strings.Replace(soBody, `"namespace":"payments"`, `"namespace":"data"`, 1), scopedop, 400},
		{"restricted operator preview own ns", "POST", "/api/v1/scaledobjects/payments/payments-api/edit/preview", soBody, scopedop, 200},
		{"restricted operator put own ns", "PUT", "/api/v1/scaledobjects/payments/payments-api", soBody, scopedop, 200},
		{"anonymous get", "GET", "/api/v1/vpas/data/redis-vpa/edit", "", func(*http.Request) {}, 401},
	}
	for _, c := range cases {
		w := e.do(c.method, c.path, c.body, c.as)
		if w.Code != c.want {
			t.Errorf("%s: got %d, want %d (%s)", c.name, w.Code, c.want, w.Body.String())
		}
	}
}

// Credentials pasted into an edit body are refused without being echoed, and
// never reach the audit log.
func TestAutoscalerEditNoSecretEcho(t *testing.T) {
	e := newAutoscalerCreateEnv(t, platform.NewDemoProvider())
	oper := withBasic("oper1", "oper-password")
	const secret = "s3cr3t-hunter2"
	var ed platform.ScaledObjectEditable
	decodeInto(t, e.do("GET", "/api/v1/scaledobjects/payments/payments-api/edit", "", oper), &ed)
	for _, mut := range []func(map[string]any){
		func(m map[string]any) {
			m["triggers"].([]any)[1].(map[string]any)["settings"].(map[string]any)["serverAddress"] = "http://admin:" + secret + "@p:9090"
		},
		func(m map[string]any) {
			m["triggers"].([]any)[1].(map[string]any)["settings"].(map[string]any)["password"] = secret
		},
	} {
		body := editBodyJSON(t, ed, mut)
		for _, path := range []string{"/api/v1/scaledobjects/payments/payments-api/edit/preview", "/api/v1/scaledobjects/payments/payments-api"} {
			method := "POST"
			if !strings.HasSuffix(path, "preview") {
				method = "PUT"
			}
			w := e.do(method, path, body, oper)
			if w.Code != 400 || strings.Contains(w.Body.String(), secret) {
				t.Errorf("%s: %d %s", path, w.Code, w.Body.String())
			}
		}
	}
	for _, a := range platform.ListAxiomAudit() {
		if strings.Contains(a.Detail, secret) || strings.Contains(a.Detail, "prometheus.monitoring") {
			t.Errorf("audit leaks: %+v", a)
		}
	}
}
