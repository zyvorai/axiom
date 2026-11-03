package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/zyvorai/axiom/internal/platform"
)

func newAutoscalerCreateEnv(t *testing.T, provider platform.Provider) *idEnv {
	t.Helper()
	hub := platform.NewUserHub(nil)
	h := NewWithUsers(provider, "demo", idAdminUser, idAdminPass, "", nil, hub,
		AuthOptions{SessionSecret: "test-secret", RevocationStore: &memRevStore{}})
	e := &idEnv{h: h, users: hub}
	e.addUser(t, "viewer1", "viewer-password", "viewer")
	e.addUser(t, "oper1", "oper-password", "operator")
	e.addUser(t, "scoped", "scoped-password", "viewer", "payments")
	e.addUser(t, "scopedop", "scopedop-password", "operator", "payments")
	return e
}

// noCRDProvider is the demo provider on a cluster without the VPA / KEDA CRDs.
type noCRDProvider struct{ *platform.DemoProvider }

var errNoCRD = platform.NewStatusError(http.StatusUnprocessableEntity, errors.New("KEDA is not installed: no scaledobjects.keda.sh CRD"))

func (noCRDProvider) CreateVPA(context.Context, platform.VPACreateRequest) (platform.AutoscalerCreateResult, error) {
	return platform.AutoscalerCreateResult{}, errNoCRD
}
func (noCRDProvider) CreateScaledObject(context.Context, platform.ScaledObjectCreateRequest) (platform.AutoscalerCreateResult, error) {
	return platform.AutoscalerCreateResult{}, errNoCRD
}
func (noCRDProvider) DeleteVPA(context.Context, string, string) error { return errNoCRD }

const (
	vpaBodyJSON = `{"namespace":"commerce","name":"checkout-rec","targetKind":"Deployment","targetName":"checkout","updateMode":"Off","controlledResources":["cpu","memory"],` +
		`"containers":[{"name":"app","minCpu":"50m","maxCpu":"2","minMemory":"64Mi","maxMemory":"2Gi"}]}`
	soBodyJSON = `{"namespace":"payments","name":"payments-api-scaler","targetKind":"Deployment","targetName":"payments-api","minReplicas":0,"maxReplicas":20,"pollingInterval":30,"cooldownPeriod":300,` +
		`"triggers":[{"type":"cron","name":"business-hours","settings":{"timezone":"Europe/Berlin","start":"0 8 * * 1-5","end":"0 18 * * 1-5","desiredReplicas":"5"}},` +
		`{"type":"prometheus","settings":{"serverAddress":"http://prometheus.monitoring:9090","query":"sum(rate(http_requests_total[2m]))","threshold":"100"},"authenticationRef":"prom-auth"}]}`
)

func hasAudit(action, target string) bool {
	for _, a := range platform.ListAxiomAudit() {
		if a.Action == action && a.Target == target {
			return true
		}
	}
	return false
}

func TestVPACreateEndpoints(t *testing.T) {
	e := newAutoscalerCreateEnv(t, platform.NewDemoProvider())
	oper := withBasic("oper1", "oper-password")

	w := e.do("POST", "/api/v1/vpas/preview", vpaBodyJSON, oper)
	if w.Code != 200 {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}
	var p platform.AutoscalerPreview
	decodeInto(t, w, &p)
	if len(p.Objects) != 1 || p.Objects[0].Kind != "VerticalPodAutoscaler" || !strings.Contains(p.YAML, "kind: VerticalPodAutoscaler") ||
		!strings.Contains(w.Body.String(), `"warnings":[]`) {
		t.Fatalf("preview body: %s", w.Body.String())
	}
	auto := strings.Replace(vpaBodyJSON, `"Off"`, `"Auto"`, 1)
	decodeInto(t, e.do("POST", "/api/v1/vpas/preview", auto, oper), &p)
	if len(p.Warnings) != 1 || !strings.Contains(p.Warnings[0], "pods may be restarted to apply new requests") {
		t.Fatalf("Auto preview warnings: %v", p.Warnings)
	}
	// Preview is pure: nothing was created.
	var list platform.VPAList
	decodeInto(t, e.do("GET", "/api/v1/vpas?namespace=commerce", "", oper), &list)
	for _, v := range list.Items {
		if v.Name == "checkout-rec" {
			t.Fatal("preview created an object")
		}
	}

	w = e.do("POST", "/api/v1/vpas", vpaBodyJSON, oper)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	t.Cleanup(func() { e.do("DELETE", "/api/v1/vpas/commerce/checkout-rec", "", asAdmin()) })
	var res platform.AutoscalerCreateResult
	decodeInto(t, w, &res)
	if !res.OK || len(res.Created) != 1 || res.Created[0].Name != "checkout-rec" {
		t.Fatalf("create body: %s", w.Body.String())
	}
	// The demo commerce/checkout HPA scales on cpu: create reports the conflict.
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "VPA and HPA on the same resource conflict") {
		t.Fatalf("create warnings: %v", res.Warnings)
	}
	if !hasAudit("autoscale.vpa_create", "commerce/checkout-rec") {
		t.Errorf("no vpa_create audit: %+v", platform.ListAxiomAudit())
	}
	decodeInto(t, e.do("GET", "/api/v1/vpas?namespace=commerce", "", oper), &list)
	found := false
	for _, v := range list.Items {
		found = found || v.Name == "checkout-rec"
	}
	if !found {
		t.Fatalf("not listed: %+v", list.Items)
	}
	if w := e.do("POST", "/api/v1/vpas", vpaBodyJSON, oper); w.Code != http.StatusConflict {
		t.Fatalf("duplicate: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("POST", "/api/v1/vpas", strings.Replace(vpaBodyJSON, `"2Gi"`, `"lots"`, 1), oper); w.Code != 400 {
		t.Fatalf("bad quantity: %d", w.Code)
	}
	if w := e.do("POST", "/api/v1/vpas", `{"namespace":"commerce","yaml":"kind: Pod"}`, oper); w.Code != 400 {
		t.Fatalf("unknown field (raw YAML) must be refused: %d", w.Code)
	}

	w = e.do("DELETE", "/api/v1/vpas/commerce/checkout-rec", "", oper)
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"ok":true}` {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	if !hasAudit("autoscale.vpa_delete", "commerce/checkout-rec") {
		t.Errorf("no vpa_delete audit")
	}
	if w := e.do("DELETE", "/api/v1/vpas/commerce/checkout-rec", "", oper); w.Code != 404 {
		t.Fatalf("delete unknown: %d", w.Code)
	}
}

func TestAutoscalerCreateRoles(t *testing.T) {
	e := newAutoscalerCreateEnv(t, platform.NewDemoProvider())
	viewer := withBasic("viewer1", "viewer-password")
	scoped := withBasic("scoped", "scoped-password")
	scopedop := withBasic("scopedop", "scopedop-password")
	payVPA := `{"namespace":"payments","name":"payments-rec","targetKind":"Deployment","targetName":"payments-api"}`
	cases := []struct {
		name, method, path, body string
		as                       reqOpt
		want                     int
	}{
		{"viewer create vpa", "POST", "/api/v1/vpas", vpaBodyJSON, viewer, 403},
		{"viewer preview vpa", "POST", "/api/v1/vpas/preview", vpaBodyJSON, viewer, 403},
		{"viewer create so", "POST", "/api/v1/scaledobjects", soBodyJSON, viewer, 403},
		{"viewer preview so", "POST", "/api/v1/scaledobjects/preview", soBodyJSON, viewer, 403},
		{"viewer delete so", "DELETE", "/api/v1/scaledobjects/data/redis-consumer", "", viewer, 403},
		{"restricted viewer preview own ns", "POST", "/api/v1/scaledobjects/preview", soBodyJSON, scoped, 403},
		{"restricted operator preview vpa other ns", "POST", "/api/v1/vpas/preview", vpaBodyJSON, scopedop, 403},
		{"restricted operator create vpa other ns", "POST", "/api/v1/vpas", vpaBodyJSON, scopedop, 403},
		{"restricted operator create vpa other ns via query", "POST", "/api/v1/vpas?namespace=payments", vpaBodyJSON, scopedop, 403},
		{"restricted operator create so other ns", "POST", "/api/v1/scaledobjects", strings.Replace(soBodyJSON, `"namespace":"payments"`, `"namespace":"data"`, 1), scopedop, 403},
		{"restricted operator delete vpa other ns", "DELETE", "/api/v1/vpas/commerce/checkout-vpa", "", scopedop, 403},
		{"restricted operator delete so other ns", "DELETE", "/api/v1/scaledobjects/data/redis-consumer", "", scopedop, 403},
		{"restricted operator preview vpa own ns", "POST", "/api/v1/vpas/preview", payVPA, scopedop, 200},
		{"restricted operator preview so own ns", "POST", "/api/v1/scaledobjects/preview", soBodyJSON, scopedop, 200},
		{"restricted operator create vpa own ns", "POST", "/api/v1/vpas", payVPA, scopedop, 201},
		{"restricted operator delete vpa own ns", "DELETE", "/api/v1/vpas/payments/payments-rec", "", scopedop, 200},
		{"restricted operator create so own ns", "POST", "/api/v1/scaledobjects", soBodyJSON, scopedop, 201},
		{"restricted operator delete so own ns", "DELETE", "/api/v1/scaledobjects/payments/payments-api-scaler", "", scopedop, 200},
		{"admin create so", "POST", "/api/v1/scaledobjects", soBodyJSON, asAdmin(), 201},
		{"admin delete so", "DELETE", "/api/v1/scaledobjects/payments/payments-api-scaler", "", asAdmin(), 200},
		// The HPA create handler now checks the body namespace too.
		{"restricted operator create hpa other ns via query", "POST", "/api/v1/hpas?namespace=payments",
			`{"namespace":"commerce","name":"sneaky","targetKind":"Deployment","targetName":"checkout"}`, scopedop, 403},
		{"anonymous preview", "POST", "/api/v1/vpas/preview", vpaBodyJSON, func(*http.Request) {}, 401},
	}
	for _, c := range cases {
		w := e.do(c.method, c.path, c.body, c.as)
		if w.Code != c.want {
			t.Errorf("%s: got %d, want %d (%s)", c.name, w.Code, c.want, w.Body.String())
		}
	}
}

func TestScaledObjectCreateEndpoints(t *testing.T) {
	e := newAutoscalerCreateEnv(t, platform.NewDemoProvider())
	oper := withBasic("oper1", "oper-password")

	w := e.do("POST", "/api/v1/scaledobjects/preview", soBodyJSON, oper)
	if w.Code != 200 {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}
	var p platform.AutoscalerPreview
	decodeInto(t, w, &p)
	if p.Objects[0].APIVersion != "keda.sh/v1alpha1" || !strings.Contains(p.YAML, "minReplicaCount: 0") || !strings.Contains(p.YAML, "name: prom-auth") {
		t.Fatalf("preview: %+v", p)
	}
	// Preview skips the TriggerAuthentication existence check; create does not.
	missing := strings.Replace(soBodyJSON, `"prom-auth"`, `"nope-auth"`, 1)
	if w := e.do("POST", "/api/v1/scaledobjects/preview", missing, oper); w.Code != 200 {
		t.Fatalf("preview with unknown auth: %d", w.Code)
	}
	w = e.do("POST", "/api/v1/scaledobjects", missing, oper)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "TriggerAuthentication nope-auth not found in namespace payments") {
		t.Fatalf("create with unknown auth: %d %s", w.Code, w.Body.String())
	}

	w = e.do("POST", "/api/v1/scaledobjects", soBodyJSON, oper)
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"warnings":[]`) {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	t.Cleanup(func() { e.do("DELETE", "/api/v1/scaledobjects/payments/payments-api-scaler", "", asAdmin()) })
	if !hasAudit("autoscale.keda_create", "payments/payments-api-scaler") {
		t.Errorf("no keda_create audit")
	}
	var list platform.ScaledObjectList
	decodeInto(t, e.do("GET", "/api/v1/scaledobjects?namespace=payments", "", oper), &list)
	found := false
	for _, s := range list.Items {
		found = found || (s.Name == "payments-api-scaler" && len(s.Triggers) == 2)
	}
	if !found {
		t.Fatalf("not listed: %+v", list.Items)
	}
	if w := e.do("POST", "/api/v1/scaledobjects", soBodyJSON, oper); w.Code != 409 {
		t.Fatalf("duplicate: %d", w.Code)
	}

	// Plain HPA on the same workload (demo commerce/checkout): create warns.
	cronOnly := `{"namespace":"commerce","name":"checkout-scaler","targetKind":"Deployment","targetName":"checkout","maxReplicas":5,` +
		`"triggers":[{"type":"cron","settings":{"timezone":"UTC","start":"0 8 * * *","end":"0 18 * * *","desiredReplicas":"3"}}]}`
	w = e.do("POST", "/api/v1/scaledobjects", cronOnly, oper)
	if w.Code != 201 || !strings.Contains(w.Body.String(), "KEDA creates its own HPA; two autoscalers on one workload fight each other") {
		t.Fatalf("hpa warning: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("DELETE", "/api/v1/scaledobjects/commerce/checkout-scaler", "", oper); w.Code != 200 {
		t.Fatalf("delete: %d", w.Code)
	}
	if !hasAudit("autoscale.keda_delete", "commerce/checkout-scaler") {
		t.Errorf("no keda_delete audit")
	}
	if w := e.do("DELETE", "/api/v1/scaledobjects/commerce/checkout-scaler", "", oper); w.Code != 404 {
		t.Fatalf("delete unknown: %d", w.Code)
	}

	// TriggerAuthentication names for the dropdown.
	w = e.do("GET", "/api/v1/triggerauthentications?namespace=payments", "", withBasic("scoped", "scoped-password"))
	var tas platform.TriggerAuthenticationList
	decodeInto(t, w, &tas)
	if w.Code != 200 || !tas.Installed || len(tas.Items) == 0 || tas.Items[0].Namespace != "payments" {
		t.Fatalf("trigger auths: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("GET", "/api/v1/triggerauthentications?namespace=data", "", withBasic("scoped", "scoped-password")); w.Code != 403 {
		t.Fatalf("restricted other ns: %d", w.Code)
	}
}

func TestScaledObjectCredentialsNeverEchoed(t *testing.T) {
	e := newAutoscalerCreateEnv(t, platform.NewDemoProvider())
	oper := withBasic("oper1", "oper-password")
	const secret = "s3cr3t-hunter2"
	bodies := map[string]string{
		"password key": `{"namespace":"payments","name":"x","targetKind":"Deployment","targetName":"payments-api","maxReplicas":3,` +
			`"triggers":[{"type":"prometheus","settings":{"serverAddress":"http://p:9090","query":"up","threshold":"1","password":"` + secret + `"}}]}`,
		"userinfo URL": `{"namespace":"payments","name":"x","targetKind":"Deployment","targetName":"payments-api","maxReplicas":3,` +
			`"triggers":[{"type":"prometheus","settings":{"serverAddress":"http://admin:` + secret + `@p:9090","query":"up","threshold":"1"}}]}`,
		"rabbitmq host": `{"namespace":"payments","name":"x","targetKind":"Deployment","targetName":"payments-api","maxReplicas":3,` +
			`"triggers":[{"type":"rabbitmq","authenticationRef":"rabbitmq-auth","settings":{"queueName":"q","mode":"QueueLength","value":"5","host":"amqp://u:` + secret + `@rabbit:5672"}}]}`,
		"unknown type": `{"namespace":"payments","name":"x","targetKind":"Deployment","targetName":"payments-api","maxReplicas":3,` +
			`"triggers":[{"type":"` + secret + `","settings":{"a":"b"}}]}`,
	}
	for name, b := range bodies {
		for _, path := range []string{"/api/v1/scaledobjects/preview", "/api/v1/scaledobjects"} {
			w := e.do("POST", path, b, oper)
			if w.Code != 400 {
				t.Errorf("%s %s: got %d %s", name, path, w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), secret) {
				t.Errorf("%s %s echoes the credential: %s", name, path, w.Body.String())
			}
		}
	}
	for _, a := range platform.ListAxiomAudit() {
		if strings.Contains(a.Target+a.Detail, secret) || strings.Contains(a.Target+a.Detail, "prometheus.monitoring") {
			t.Errorf("audit leaks settings: %+v", a)
		}
	}
}

func TestAutoscalerPreviewWithoutCRD(t *testing.T) {
	e := newAutoscalerCreateEnv(t, noCRDProvider{platform.NewDemoProvider()})
	oper := withBasic("oper1", "oper-password")
	if w := e.do("POST", "/api/v1/vpas/preview", vpaBodyJSON, oper); w.Code != 200 {
		t.Fatalf("vpa preview without CRD: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("POST", "/api/v1/scaledobjects/preview", soBodyJSON, oper); w.Code != 200 {
		t.Fatalf("so preview without CRD: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("POST", "/api/v1/vpas", vpaBodyJSON, oper); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("vpa create without CRD: %d", w.Code)
	}
	if w := e.do("POST", "/api/v1/scaledobjects", soBodyJSON, oper); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "not installed") {
		t.Fatalf("so create without CRD: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("DELETE", "/api/v1/vpas/commerce/checkout-vpa", "", oper); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("vpa delete without CRD: %d", w.Code)
	}
}
