//go:build kubernetes

package platform

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func helmRevSecret(t *testing.T, rev int, status string, manifest string) *corev1.Secret {
	t.Helper()
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sh.helm.release.v1.web.v" + string(rune('0'+rev)), Namespace: "apps",
			Labels: map[string]string{"owner": "helm", "name": "web", "version": string(rune('0' + rev)), "status": status},
		},
		Type: "helm.sh/release.v1",
		Data: map[string][]byte{"release": encodeRel(t, relFixture(rev, status, manifest, map[string]any{"adminPassword": "s3cret"}, 0), true)},
	}
}

func TestKubernetesHelmRollbackReviewed(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(
		helmRevSecret(t, 1, "superseded", "kind: Deployment\nimage: v1"),
		helmRevSecret(t, 2, "deployed", "kind: Deployment\nimage: v2\n---\nkind: PersistentVolumeClaim"),
	)
	revs, err := k.ListHelmRevisions(ctx, "apps", "web")
	if err != nil || len(revs) != 2 || revs[0].Revision != 2 {
		t.Fatalf("revisions: %+v %v", revs, err)
	}
	d, err := k.DiffHelmRevisions(ctx, "apps", "web", 2, 1)
	if err != nil || !d.Changed {
		t.Fatalf("diff: %+v %v", d, err)
	}
	p, err := k.PreviewHelmRollback(ctx, "apps", "web", 0)
	if err != nil || p.Target != 1 || p.Current != 2 || len(p.Warnings) != 1 || p.Warnings[0] != "pvc" {
		t.Fatalf("preview: %+v %v", p, err)
	}
	if _, err := k.RollbackHelm(ctx, HelmRollbackRequest{Namespace: "apps", Name: "web", Revision: 1, Confirm: "nope"}); err == nil {
		t.Fatal("rollback without typed confirmation must fail")
	}
	if jobs, _ := cs.BatchV1().Jobs("axiom-system").List(ctx, metav1.ListOptions{}); len(jobs.Items) != 0 {
		t.Fatal("job created without confirmation")
	}
	res, err := k.RollbackHelm(ctx, HelmRollbackRequest{Namespace: "apps", Name: "web", Revision: 1, Confirm: "web"})
	if err != nil || !res.OK {
		t.Fatalf("rollback: %+v %v", res, err)
	}
	jobs, _ := cs.BatchV1().Jobs("axiom-system").List(ctx, metav1.ListOptions{})
	if len(jobs.Items) != 1 || strings.Join(jobs.Items[0].Spec.Template.Spec.Containers[0].Command, " ") != "helm rollback web 1 -n apps --wait" {
		t.Fatalf("jobs: %+v", jobs.Items)
	}
}
