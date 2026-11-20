package platform

import (
	"context"
	"net/http"
	"testing"
)

func TestDemoSuspendAndAppSetSync(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	if err := d.TriggerGitOpsSync(ctx, "ApplicationSet", "argocd", "tenant-apps"); HTTPStatus(err) != http.StatusUnprocessableEntity {
		t.Fatalf("appset sync: %v", err)
	}
	res, err := d.SuspendGitOpsApp(ctx, "Kustomization", "flux-system", "apps", GitOpsSuspendRequest{Suspend: true, ResourceVersion: "1"})
	if err != nil || !res.OK || !res.Changed {
		t.Fatalf("suspend %+v %v", res, err)
	}
	items, _ := d.ListGitOps(ctx)
	var apps GitOpsItem
	for _, it := range items {
		if it.Name == "apps" && it.Kind == "Kustomization" {
			apps = it
		}
		if it.Kind == "ApplicationSet" && (it.CanSync || it.CanEdit) {
			t.Fatalf("appset must not sync/edit: %+v", it)
		}
	}
	if !apps.Suspended {
		t.Fatalf("expected suspended: %+v", apps)
	}
	res, err = d.SuspendGitOpsApp(ctx, "Kustomization", "flux-system", "apps", GitOpsSuspendRequest{Suspend: false, ResourceVersion: "1"})
	if err != nil || !res.Changed {
		t.Fatalf("resume %+v %v", res, err)
	}
}
