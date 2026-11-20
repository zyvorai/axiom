package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDemoDiffManifestNewObjectIsAllAddition(t *testing.T) {
	d := NewDemoProvider()
	res, err := d.DiffManifest(context.Background(), "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: brand-new\n  namespace: apps\n")
	if err != nil || !res.Valid {
		t.Fatalf("diff: valid=%v err=%v errors=%v", res.Valid, err, res.Errors)
	}
	o := res.Objects[0]
	if o.Exists || o.Live != "" || !o.Changed || o.Name != "brand-new" || o.Namespace != "apps" {
		t.Fatalf("unexpected object: %+v", o)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("a new object must not warn: %v", res.Warnings)
	}
}

func TestDemoDiffManifestExistingObjectDetectsEditsAndForeignManagers(t *testing.T) {
	ctx := context.Background()
	d := NewDemoProvider()
	detail, err := d.GetResource(ctx, "payments", "Deployment", "payments-api")
	if err != nil {
		t.Fatal(err)
	}

	// Unchanged manifest -> no diff, but the Argo CD ownership warning is still raised.
	same, _ := json.Marshal(detail.Manifest)
	res, err := d.DiffManifest(ctx, string(same))
	if err != nil || !res.Valid {
		t.Fatalf("diff: %v %v", err, res.Errors)
	}
	o := res.Objects[0]
	if !o.Exists || o.Changed {
		t.Fatalf("identical manifest should be unchanged: exists=%v changed=%v\nlive=%s\nproposed=%s", o.Exists, o.Changed, o.Live, o.Proposed)
	}
	if len(o.ForeignManagers) != 1 || o.ForeignManagers[0] != "argocd-application-controller" {
		t.Fatalf("foreign managers = %v", o.ForeignManagers)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "argocd-application-controller") {
		t.Fatalf("expected an ownership warning, got %v", res.Warnings)
	}

	// Editing a label -> changed, with the edit visible in the proposed text only.
	edited := map[string]any{}
	_ = json.Unmarshal(same, &edited)
	edited["metadata"].(map[string]any)["labels"] = map[string]any{"edited": "yes"}
	b, _ := json.Marshal(edited)
	res, _ = d.DiffManifest(ctx, string(b))
	o = res.Objects[0]
	if !o.Changed || !strings.Contains(o.Proposed, `"edited": "yes"`) || strings.Contains(o.Live, "edited") {
		t.Fatalf("edit not reflected: %+v", o)
	}
}

func TestDemoDiffManifestRejectsBadInput(t *testing.T) {
	d := NewDemoProvider()
	for name, in := range map[string]string{
		"empty":     "  ",
		"no kind":   "metadata:\n  name: x\n",
		"no name":   "apiVersion: v1\nkind: ConfigMap\n",
		"multi-doc": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a\n---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: b\n",
	} {
		res, err := d.DiffManifest(context.Background(), in)
		if err != nil || res.Valid || len(res.Errors) == 0 {
			t.Errorf("%s: want invalid with errors, got valid=%v errors=%v err=%v", name, res.Valid, res.Errors, err)
		}
	}
}

func TestDemoDiffManifestNeverEchoesSecretValues(t *testing.T) {
	d := NewDemoProvider()
	for _, in := range []string{
		`{"apiVersion":"v1","kind":"Secret","metadata":{"name":"s","namespace":"default"},"data":{"password":"c3VwZXItc2VjcmV0"},"stringData":{"token":"hunter2"}}`,
		"apiVersion: v1\nkind: Secret\nmetadata:\n  name: s\n  namespace: default\nstringData:\n  token: hunter2\n",
	} {
		res, err := d.DiffManifest(context.Background(), in)
		if err != nil || !res.Valid {
			t.Fatalf("diff: %v %v", err, res.Errors)
		}
		out, _ := json.Marshal(res)
		for _, leak := range []string{"hunter2", "c3VwZXItc2VjcmV0"} {
			if strings.Contains(string(out), leak) {
				t.Fatalf("secret value %q leaked into the diff: %s", leak, out)
			}
		}
	}
}

func TestDemoResourceGraphWorkload(t *testing.T) {
	g, err := NewDemoProvider().ResourceGraph(context.Background(), "payments", "Deployment", "payments-api")
	if err != nil {
		t.Fatal(err)
	}
	if g.Root != "deployment/payments/payments-api" {
		t.Fatalf("root = %q", g.Root)
	}
	kinds := map[string]int{}
	ids := map[string]bool{}
	for _, n := range g.Nodes {
		kinds[n.Kind]++
		ids[n.ID] = true
	}
	for _, k := range []string{"Deployment", "ReplicaSet", "Pod", "Service", "Ingress"} {
		if kinds[k] == 0 {
			t.Errorf("graph has no %s node: %+v", k, kinds)
		}
	}
	if kinds["Pod"] != 12 {
		t.Errorf("want 12 pods, got %d", kinds["Pod"])
	}
	rel := map[string]bool{}
	for _, e := range g.Edges {
		if !ids[e.From] || !ids[e.To] {
			t.Errorf("dangling edge %+v", e)
		}
		rel[e.Relation] = true
	}
	for _, r := range []string{"owns", "selects", "routes"} {
		if !rel[r] {
			t.Errorf("missing %q edge", r)
		}
	}
}

func TestDemoResourceGraphPodAndMissing(t *testing.T) {
	d := NewDemoProvider()
	g, err := d.ResourceGraph(context.Background(), "commerce", "pod", "checkout-01")
	if err != nil {
		t.Fatal(err)
	}
	if g.Root != "pod/commerce/checkout-01" {
		t.Fatalf("root = %q", g.Root)
	}
	if _, err := d.ResourceGraph(context.Background(), "commerce", "Deployment", "nope"); err == nil {
		t.Fatal("expected not found")
	} else if HTTPStatus(err) != 404 {
		t.Fatalf("status = %d", HTTPStatus(err))
	}
}

func TestForeignFieldManagerClassification(t *testing.T) {
	for m, want := range map[string]bool{
		"zyvor-axiom": false, "kube-controller-manager": false, "kubelet": false, "": false,
		"argocd-controller": true, "kustomize-controller": true, "helm": true, "kubectl-client-side-apply": true,
	} {
		if got := isForeignFieldManager(m); got != want {
			t.Errorf("isForeignFieldManager(%q) = %v, want %v", m, got, want)
		}
	}
	if foreignManagerWarning("Deployment", "x", nil) != "" {
		t.Error("no managers must produce no warning")
	}
}
