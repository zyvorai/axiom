//go:build kubernetes

package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	clienttesting "k8s.io/client-go/testing"
)

func TestHelmJobNameIsDNSSafeAndAtMost63(t *testing.T) {
	long := strings.Repeat("a", 80)
	for _, action := range []string{"install", "upgrade", "uninstall", "rollback"} {
		n := helmJobName(action, long)
		if len(n) > 63 {
			t.Errorf("%s: job name %d chars > 63: %q", action, len(n), n)
		}
		if n != strings.ToLower(n) || strings.ContainsAny(n, " _./;") || strings.HasSuffix(n, "-") {
			t.Errorf("%s: not DNS-1123 safe: %q", action, n)
		}
	}
	if a, b := helmJobName("install", "web"), helmJobName("install", "web"); a == "" || b == "" {
		t.Fatal("empty job name")
	}
}

func TestHelmRejectsFlagLikeArguments(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	req := InstallHelmRequest{Namespace: "apps", ReleaseName: "web", RepoURL: "https://r.example", ChartName: "c"}
	for field, mutate := range map[string]func(*InstallHelmRequest){
		"release": func(r *InstallHelmRequest) { r.ReleaseName = "--post-renderer=/bin/sh" },
		"chart":   func(r *InstallHelmRequest) { r.ChartName = "--kubeconfig=/x" },
		"repo":    func(r *InstallHelmRequest) { r.RepoURL = "-v" },
		"version": func(r *InstallHelmRequest) { r.ChartVersion = "--devel" },
		"ns":      func(r *InstallHelmRequest) { r.Namespace = "-A" },
	} {
		r := req
		mutate(&r)
		if _, err := k.InstallHelmChart(ctx, r); err == nil {
			t.Errorf("install: %s starting with '-' must be rejected", field)
		}
		if _, err := k.UpgradeHelmChart(ctx, r); err == nil {
			t.Errorf("upgrade: %s starting with '-' must be rejected", field)
		}
	}
	if _, err := k.HelmUninstall(ctx, "apps", "--wait=false"); err == nil {
		t.Error("uninstall: flag-like release must be rejected")
	}
	jobs, _ := cs.BatchV1().Jobs(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if len(jobs.Items) != 0 {
		t.Fatalf("rejected requests must not create jobs, got %d", len(jobs.Items))
	}
}

func TestHelmJobUsesConfiguredNamespaceServiceAccountAndImage(t *testing.T) {
	t.Setenv("AXIOM_NAMESPACE", "platform-ns")
	t.Setenv("AXIOM_SERVICE_ACCOUNT", "axiom-sa")
	t.Setenv("AXIOM_HELM_IMAGE", "registry.internal/helm:3.15")
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	if _, err := k.HelmUninstall(ctx, "apps", "web"); err != nil {
		t.Fatal(err)
	}
	jobs, _ := cs.BatchV1().Jobs("platform-ns").List(ctx, metav1.ListOptions{})
	if len(jobs.Items) != 1 {
		t.Fatalf("job must be created in the configured Axiom namespace, got %d", len(jobs.Items))
	}
	spec := jobs.Items[0].Spec.Template.Spec
	if spec.ServiceAccountName != "axiom-sa" || spec.Containers[0].Image != "registry.internal/helm:3.15" {
		t.Fatalf("unexpected SA/image: %q %q", spec.ServiceAccountName, spec.Containers[0].Image)
	}
}

func TestHelmValuesSecretIsOwnedByTheJob(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	_, _, name, err := k.createHelmValuesSecret(ctx, "axiom-system", "axiom-helm-install-x-1", "pw: hunter2")
	if err != nil {
		t.Fatal(err)
	}
	k.adoptHelmValuesSecret(ctx, "axiom-system", name, &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "axiom-helm-install-x-1", UID: "uid-1"}})
	sec, _ := cs.CoreV1().Secrets("axiom-system").Get(ctx, name, metav1.GetOptions{})
	if len(sec.OwnerReferences) != 1 || sec.OwnerReferences[0].UID != "uid-1" || sec.OwnerReferences[0].Kind != "Job" {
		t.Fatalf("values secret must be owned by the job so it is garbage-collected: %+v", sec.OwnerReferences)
	}
}

func TestFitAuditEntriesStaysUnderLimitKeepingNewest(t *testing.T) {
	var entries []AuditEntry
	for i := 0; i < 6000; i++ {
		entries = append(entries, AuditEntry{Action: "workload.restart", Target: fmt.Sprintf("ns/deploy-%05d", i), Detail: strings.Repeat("x", 120), User: "alice"})
	}
	raw, kept, err := fitAuditEntries(entries, maxAuditPersistBytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > maxAuditPersistBytes {
		t.Fatalf("persisted payload %d bytes exceeds the limit %d", len(raw), maxAuditPersistBytes)
	}
	if len(kept) == 0 || len(kept) >= len(entries) {
		t.Fatalf("expected the oldest entries to be dropped, kept %d of %d", len(kept), len(entries))
	}
	if kept[len(kept)-1].Target != entries[len(entries)-1].Target {
		t.Fatal("the NEWEST entry must be kept")
	}
	var round []AuditEntry
	if err := json.Unmarshal(raw, &round); err != nil || len(round) != len(kept) {
		t.Fatalf("payload must round-trip: %v", err)
	}
	// small rings are untouched
	_, small, _ := fitAuditEntries(entries[:10], maxAuditPersistBytes)
	if len(small) != 10 {
		t.Fatalf("small ring must not be trimmed, got %d", len(small))
	}
}

func helmSecret(ns, release string, rev int) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("sh.helm.release.v1.%s.v%d", release, rev), Namespace: ns,
			Labels: map[string]string{"owner": "helm", "name": release, "version": fmt.Sprint(rev), "status": "deployed"},
		},
		Type: "helm.sh/release.v1",
	}
}

func TestListHelmReleasesIgnoresOtherSecretsAndPicksNewestRevision(t *testing.T) {
	ctx := context.Background()
	k, _ := newFakeKubernetesProvider(
		helmSecret("apps", "web", 1), helmSecret("apps", "web", 3), helmSecret("apps", "web", 2),
		helmSecret("data", "db", 1),
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "tls", Namespace: "apps"}, Data: map[string][]byte{"tls.key": []byte("secret")}},
	)
	rels, err := k.ListHelmReleases(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 2 {
		t.Fatalf("want 2 releases (web, db), got %+v", rels)
	}
	for _, r := range rels {
		if r.Name == "web" && r.Revision != 3 {
			t.Fatalf("web must resolve to its newest revision 3, got %d", r.Revision)
		}
	}
}

func TestLimitedBufferCapsOutput(t *testing.T) {
	b := &limitedBuffer{max: 10}
	n, err := b.Write([]byte(strings.Repeat("a", 25)))
	if err != nil || n != 25 {
		t.Fatalf("Write must report all bytes consumed so the stream keeps draining: n=%d err=%v", n, err)
	}
	if b.Len() != 10 || !strings.Contains(b.String(), "truncated") {
		t.Fatalf("output must be capped and flagged: len=%d %q", b.Len(), b.String())
	}
	b.Write([]byte("more")) // further writes are swallowed
	if b.Len() != 10 {
		t.Fatalf("buffer grew past its cap: %d", b.Len())
	}
}

func TestExecSessionCap(t *testing.T) {
	var releases []func()
	for i := 0; i < maxExecWSSessions; i++ {
		rel, ok := acquireExecWSSession()
		if !ok {
			t.Fatalf("session %d should be allowed", i)
		}
		releases = append(releases, rel)
	}
	if _, ok := acquireExecWSSession(); ok {
		t.Fatal("the session beyond the cap must be refused")
	}
	releases[0]()
	if rel, ok := acquireExecWSSession(); !ok {
		t.Fatal("a freed slot must be reusable")
	} else {
		rel()
	}
	for _, r := range releases[1:] {
		r()
	}
}

func TestBearerTokenForRereadsTheTokenFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "token")
	os.WriteFile(f, []byte("old\n"), 0o600)
	cfg := &rest.Config{BearerToken: "frozen", BearerTokenFile: f}
	if got := bearerTokenFor(cfg); got != "old" {
		t.Fatalf("got %q", got)
	}
	os.WriteFile(f, []byte("rotated\n"), 0o600)
	if got := bearerTokenFor(cfg); got != "rotated" {
		t.Fatalf("a rotated token file must be picked up, got %q", got)
	}
	if got := bearerTokenFor(&rest.Config{BearerToken: "static"}); got != "static" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyClientDefaultsAndStreamingClient(t *testing.T) {
	cfg := &rest.Config{Host: "https://example"}
	applyClientDefaults(cfg)
	if cfg.Timeout != apiRequestTimeout || cfg.QPS != apiQPS || cfg.Burst != apiBurst {
		t.Fatalf("defaults not applied: %+v", cfg)
	}
	keep := &rest.Config{Host: "https://example", Timeout: 5 * time.Second, QPS: 7, Burst: 9}
	applyClientDefaults(keep)
	if keep.Timeout != 5*time.Second || keep.QPS != 7 || keep.Burst != 9 {
		t.Fatal("explicit settings must not be overridden")
	}
	k := &KubernetesProvider{config: cfg}
	if k.streamingClient() == nil {
		t.Fatal("streamingClient must return a client")
	}
	if cfg.Timeout != apiRequestTimeout {
		t.Fatal("building the streaming client must not mutate the provider's own config")
	}
}

func TestUtilizationHistoryIsPerClusterAndHasNoInventedPoints(t *testing.T) {
	utilMu.Lock()
	utilHistory = map[string][]utilSample{}
	utilMu.Unlock()
	recordUtilization("a", 10, 20)
	recordUtilization("b", 90, 80)
	a, b := utilizationSeries("a"), utilizationSeries("b")
	if len(a) != 1 || a[0].CPU != 10 || len(b) != 1 || b[0].CPU != 90 {
		t.Fatalf("clusters must not share history: a=%+v b=%+v", a, b)
	}
	if len(utilizationSeries("none")) != 0 {
		t.Fatal("no measured samples means no points (no decorative curve)")
	}
}

func TestUndoDeploymentIsASingleUpdateWithoutPodTemplateHash(t *testing.T) {
	ctx := context.Background()
	mkRS := func(name, rev, image string) *appsv1.ReplicaSet {
		return &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: name, Namespace: "apps",
				Annotations:     map[string]string{"deployment.kubernetes.io/revision": rev},
				OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "web"}},
			},
			Spec: appsv1.ReplicaSetSpec{Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "web", "pod-template-hash": "h" + rev}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "web", Image: image}}},
			}},
			Status: appsv1.ReplicaSetStatus{Replicas: map[string]int32{"2": 0, "3": 3}[rev]},
		}
	}
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "apps"},
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "web", Image: "web:v3"}}},
		}},
	}
	k, cs := newFakeKubernetesProvider(dep, mkRS("web-2", "2", "web:v2"), mkRS("web-3", "3", "web:v3"))
	var updates int
	cs.PrependReactor("update", "deployments", func(a clienttesting.Action) (bool, runtime.Object, error) { updates++; return false, nil, nil })
	cs.PrependReactor("patch", "deployments", func(a clienttesting.Action) (bool, runtime.Object, error) {
		t.Error("undo must not patch the Deployment (the old bogus-annotation patch caused a spurious rollout)")
		return false, nil, nil
	})
	if err := k.undoDeployment(ctx, "apps", "web"); err != nil {
		t.Fatalf("undoDeployment: %v", err)
	}
	got, _ := cs.AppsV1().Deployments("apps").Get(ctx, "web", metav1.GetOptions{})
	if img := got.Spec.Template.Spec.Containers[0].Image; img != "web:v2" {
		t.Fatalf("template must be rolled back to revision 2, got image %q", img)
	}
	if _, has := got.Spec.Template.Labels["pod-template-hash"]; has {
		t.Fatal("pod-template-hash must be dropped (as `kubectl rollout undo` does)")
	}
	if updates != 1 {
		t.Fatalf("expected exactly one Update, got %d", updates)
	}
	if _, has := got.Spec.Template.Annotations["deployment.kubernetes.io/revision"]; has {
		t.Fatal("the revision annotation must not be injected into the pod template")
	}
}

func TestListAlertsRequestsWarningsServerSide(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	var selector string
	cs.PrependReactor("list", "events", func(a clienttesting.Action) (bool, runtime.Object, error) {
		selector = a.(clienttesting.ListAction).GetListRestrictions().Fields.String()
		return false, nil, nil
	})
	if _, err := k.ListAlerts(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if selector != "type=Warning" {
		t.Fatalf("alerts must ask the API server for Warning events only, field selector was %q", selector)
	}
}
