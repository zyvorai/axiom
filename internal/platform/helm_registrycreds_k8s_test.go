//go:build kubernetes

package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"
)

func readRegistryCredSecret(t *testing.T, s *K8sHelmRegistryCredStore) (*corev1.Secret, map[string]helmRegistryCred) {
	t.Helper()
	sec, err := s.client.CoreV1().Secrets(s.ns).Get(context.Background(), HelmRegistryCredSecretName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("secret: %v", err)
	}
	m := map[string]helmRegistryCred{}
	if err := json.Unmarshal(sec.Data[helmRegistryCredSecretKey], &m); err != nil {
		t.Fatalf("credentials.json: %v", err)
	}
	return sec, m
}

func TestK8sHelmRegistryCredStore(t *testing.T) {
	ctx := context.Background()
	_, cs := newFakeKubernetesProvider()
	s := NewK8sHelmRegistryCredStore(cs, "axiom-system")
	if l, err := s.List(ctx); err != nil || len(l) != 0 {
		t.Fatalf("empty: %v %v", l, err)
	}
	if _, err := s.get(ctx, "ghcr"); HTTPStatus(err) != 404 {
		t.Fatalf("get missing: %v", err)
	}
	addTestCred(t, s, "ghcr", "ghcr.io", "bot", testRegistryPassword)
	addTestCred(t, s, "quay", "quay.io:443", "robot", "pw2")
	sec, m := readRegistryCredSecret(t, s)
	if sec.Type != corev1.SecretTypeOpaque || sec.Labels["app.kubernetes.io/managed-by"] != "zyvor-axiom" || len(m) != 2 ||
		m["ghcr"].Password != testRegistryPassword || m["ghcr"].Registry != "ghcr.io" || m["quay"].Username != "robot" || m["ghcr"].CreatedAt == "" {
		t.Fatalf("stored: %v %v", sec.Labels, m)
	}
	if _, err := s.Add(ctx, HelmRegistryCredRequest{Name: "ghcr", Registry: "x.io", Username: "u", Password: "p"}); HTTPStatus(err) != 409 {
		t.Fatalf("dup: %v", err)
	}
	if _, m := readRegistryCredSecret(t, s); m["ghcr"].Registry != "ghcr.io" {
		t.Fatal("a duplicate overwrote the credential")
	}
	list, _ := s.List(ctx)
	b, _ := json.Marshal(list)
	if len(list) != 2 || strings.Contains(string(b), testRegistryPassword) {
		t.Fatalf("list: %s", b)
	}
	if c, err := s.get(ctx, "ghcr"); err != nil || c.Password != testRegistryPassword {
		t.Fatalf("get: %v", err)
	}
	// Another replica sees the same data.
	if again, _ := NewK8sHelmRegistryCredStore(cs, "axiom-system").List(ctx); len(again) != 2 {
		t.Fatalf("reload: %v", again)
	}
	if gone, err := s.Delete(ctx, "quay"); err != nil || gone.Registry != "quay.io:443" {
		t.Fatalf("delete: %+v %v", gone, err)
	}
	if _, err := s.Delete(ctx, "quay"); HTTPStatus(err) != 404 {
		t.Fatalf("delete twice: %v", err)
	}
	if _, m := readRegistryCredSecret(t, s); len(m) != 1 {
		t.Fatalf("after delete: %v", m)
	}
}

func TestK8sHelmRegistryCredStoreConflictRetry(t *testing.T) {
	_, cs := newFakeKubernetesProvider()
	s := NewK8sHelmRegistryCredStore(cs, "axiom-system")
	addTestCred(t, s, "first", "ghcr.io", "bot", "pw")
	conflicts := 0
	cs.PrependReactor("update", "secrets", func(a k8stesting.Action) (bool, runtime.Object, error) {
		if conflicts < 2 {
			conflicts++
			// Another replica wrote in between: add its credential, then conflict.
			raw, _ := json.Marshal(map[string]helmRegistryCred{
				"first": {Registry: "ghcr.io", Username: "bot", Password: "pw"},
				"other": {Registry: "quay.io", Username: "x", Password: "y"},
			})
			_ = cs.Tracker().Update(schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: HelmRegistryCredSecretName, Namespace: "axiom-system"},
				Data:       map[string][]byte{helmRegistryCredSecretKey: raw},
			}, "axiom-system")
			return true, nil, apierrors.NewConflict(schema.GroupResource{Resource: "secrets"}, HelmRegistryCredSecretName, errors.New("changed"))
		}
		return false, nil, nil
	})
	addTestCred(t, s, "second", "docker.io", "bot", "pw")
	if conflicts != 2 {
		t.Fatalf("conflicts: %d", conflicts)
	}
	if _, m := readRegistryCredSecret(t, s); len(m) != 3 || m["other"].Registry != "quay.io" || m["second"].Registry != "docker.io" {
		t.Fatalf("the concurrent write was lost: %v", m)
	}
}

func TestK8sHelmRegistryCredStoreCorruptNotOverwritten(t *testing.T) {
	ctx := context.Background()
	t.Setenv("POD_NAMESPACE", "")
	_, cs := newFakeKubernetesProvider(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: HelmRegistryCredSecretName, Namespace: "axiom-system"},
		Data:       map[string][]byte{helmRegistryCredSecretKey: []byte(`{"x":{"password":"` + testRegistryPassword + `"` /* truncated */)},
	})
	s := NewK8sHelmRegistryCredStore(cs, "")
	if s.ns != "axiom-system" {
		t.Fatalf("default namespace: %q", s.ns)
	}
	_, err := s.List(ctx)
	if err == nil || strings.Contains(err.Error(), testRegistryPassword) {
		t.Fatalf("corrupt list: %v", err)
	}
	if _, err := s.Add(ctx, HelmRegistryCredRequest{Name: "new", Registry: "ghcr.io", Username: "u", Password: "p"}); err == nil || strings.Contains(err.Error(), testRegistryPassword) {
		t.Fatalf("must not overwrite corrupt data: %v", err)
	}
	if _, err := s.Delete(ctx, "x"); err == nil {
		t.Fatal("delete over corrupt data")
	}
	sec, _ := cs.CoreV1().Secrets("axiom-system").Get(ctx, HelmRegistryCredSecretName, metav1.GetOptions{})
	if !strings.HasSuffix(string(sec.Data[helmRegistryCredSecretKey]), testRegistryPassword+`"`) {
		t.Fatalf("corrupt secret was rewritten: %q", sec.Data[helmRegistryCredSecretKey])
	}
}

func TestOpenHelmRegistryCredStoreModes(t *testing.T) {
	if _, ok := OpenHelmRegistryCredStore("demo", nil).(*MemoryHelmRegistryCredStore); !ok {
		t.Fatal("demo mode keeps credentials in memory")
	}
	p, _ := newFakeKubernetesProvider()
	if _, ok := OpenHelmRegistryCredStore("kubernetes", p).(*K8sHelmRegistryCredStore); !ok {
		t.Fatal("kubernetes mode uses the Secret store")
	}
}

// ---------------------------------------------------------------------------
// Install / upgrade Jobs

func helmJobByAction(t *testing.T, k *KubernetesProvider, action string) batchv1.Job {
	t.Helper()
	jobs, _ := k.client.BatchV1().Jobs(metav1.NamespaceAll).List(context.Background(), metav1.ListOptions{})
	for _, j := range jobs.Items {
		if j.Labels["axiom.zyvor.dev/helm-action"] == action {
			return j
		}
	}
	t.Fatalf("no %s job", action)
	return batchv1.Job{}
}

func resolvedAccess(t *testing.T, ref string) *HelmOCIAccess {
	t.Helper()
	ctx := context.Background()
	creds := NewMemoryHelmRegistryCredStore()
	repos := NewMemoryHelmRepoStore()
	addTestCred(t, creds, "ghcr", "ghcr.io", "bot", testRegistryPassword)
	_ = repos.Add(ctx, HelmRepo{Name: "org", URL: "oci://ghcr.io/org", Kind: "oci"})
	_ = repos.Add(ctx, HelmRepo{Name: "private", URL: "oci://ghcr.io/org/private", Kind: "oci", CredentialsSecret: "ghcr"})
	_ = repos.Add(ctx, HelmRepo{Name: "lab", URL: "oci://registry.lab:5000/charts", Kind: "oci", PlainHTTP: true})
	a, err := ResolveHelmOCIAccess(ctx, repos, creds, ref)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestHelmOCIInstallJobWithCredentials(t *testing.T) {
	ctx := context.Background()
	for _, action := range []string{"install", "upgrade"} {
		t.Run(action, func(t *testing.T) {
			k, cs := newFakeKubernetesProvider()
			ref := "oci://ghcr.io/org/private/nginx"
			req := InstallHelmRequest{Namespace: "apps", ReleaseName: "web", OCIRef: ref, ChartVersion: "1.2.0", Values: "a: 1\n", OCIAccess: resolvedAccess(t, ref)}
			var err error
			if action == "install" {
				_, err = k.InstallHelmChart(ctx, req)
			} else {
				_, err = k.UpgradeHelmChart(ctx, req)
			}
			if err != nil {
				t.Fatal(err)
			}
			job := helmJobByAction(t, k, action)
			c := job.Spec.Template.Spec.Containers[0]
			want := []string{"helm", action, "web", ref, "-n", "apps"}
			if action == "install" {
				want = append(want, "--create-namespace")
			}
			want = append(want, "--version", "1.2.0", "-f", "/values/values.yaml")
			if strings.Join(c.Command, "\x00") != strings.Join(want, "\x00") {
				t.Fatalf("args:\n got %q\nwant %q", c.Command, want)
			}
			if len(c.Env) != 1 || c.Env[0].Name != "HELM_REGISTRY_CONFIG" || c.Env[0].Value != "/registry/config.json" || c.Env[0].ValueFrom != nil {
				t.Fatalf("env: %+v", c.Env)
			}
			var mnt *corev1.VolumeMount
			for i := range c.VolumeMounts {
				if c.VolumeMounts[i].Name == "registry" {
					mnt = &c.VolumeMounts[i]
				}
			}
			if mnt == nil || mnt.MountPath != "/registry" || !mnt.ReadOnly {
				t.Fatalf("mounts: %+v", c.VolumeMounts)
			}
			secretName := job.Name + "-registry"
			found := false
			for _, v := range job.Spec.Template.Spec.Volumes {
				if v.Name == "registry" && v.Secret != nil && v.Secret.SecretName == secretName {
					found = true
				}
			}
			if !found {
				t.Fatalf("volumes: %+v", job.Spec.Template.Spec.Volumes)
			}
			// The password is nowhere in the Job object.
			raw, _ := json.Marshal(job)
			enc := base64.StdEncoding.EncodeToString([]byte("bot:" + testRegistryPassword))
			for _, s := range []string{testRegistryPassword, enc, base64.StdEncoding.EncodeToString([]byte(testRegistryPassword))} {
				if strings.Contains(string(raw), s) {
					t.Fatalf("the Job carries the credential (%q)", s)
				}
			}
			sec, err := cs.CoreV1().Secrets(job.Namespace).Get(ctx, secretName, metav1.GetOptions{})
			if err != nil {
				t.Fatalf("registry secret: %v", err)
			}
			var cfg struct {
				Auths map[string]struct {
					Auth string `json:"auth"`
				} `json:"auths"`
			}
			if err := json.Unmarshal(sec.Data["config.json"], &cfg); err != nil || len(cfg.Auths) != 1 || cfg.Auths["ghcr.io"].Auth != enc || len(sec.Data) != 1 {
				t.Fatalf("config.json: %s %v", sec.Data["config.json"], err)
			}
			if sec.Labels["app.kubernetes.io/managed-by"] != "zyvor-axiom" {
				t.Fatalf("labels: %v", sec.Labels)
			}
		})
	}
}

func TestHelmOCIJobRegistrySecretOwnedByJob(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	// The fake clientset assigns no UIDs: give the Job one, as the API server would.
	cs.PrependReactor("create", "jobs", func(a k8stesting.Action) (bool, runtime.Object, error) {
		j := a.(k8stesting.CreateAction).GetObject().(*batchv1.Job)
		j.UID = "job-uid-1"
		return false, nil, nil
	})
	ref := "oci://ghcr.io/org/private/nginx"
	if _, err := k.InstallHelmChart(ctx, InstallHelmRequest{Namespace: "apps", ReleaseName: "web", OCIRef: ref, OCIAccess: resolvedAccess(t, ref)}); err != nil {
		t.Fatal(err)
	}
	job := helmJobByAction(t, k, "install")
	sec, err := cs.CoreV1().Secrets(job.Namespace).Get(ctx, job.Name+"-registry", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sec.OwnerReferences) != 1 || sec.OwnerReferences[0].Kind != "Job" || sec.OwnerReferences[0].Name != job.Name || sec.OwnerReferences[0].UID != "job-uid-1" {
		t.Fatalf("owner: %+v", sec.OwnerReferences)
	}
	if job.Spec.TTLSecondsAfterFinished == nil || *job.Spec.TTLSecondsAfterFinished != 600 {
		t.Fatal("the Job (and with it the Secret) must expire")
	}
}

func TestHelmOCIJobRegistrySecretRemovedWhenJobFails(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	cs.PrependReactor("create", "jobs", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("quota exceeded")
	})
	ref := "oci://ghcr.io/org/private/nginx"
	if _, err := k.InstallHelmChart(ctx, InstallHelmRequest{Namespace: "apps", ReleaseName: "web", OCIRef: ref, OCIAccess: resolvedAccess(t, ref)}); err == nil {
		t.Fatal("want an error")
	}
	secs, _ := cs.CoreV1().Secrets(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	for _, s := range secs.Items {
		if strings.HasSuffix(s.Name, "-registry") {
			t.Fatalf("registry secret left behind: %s", s.Name)
		}
	}
}

func TestHelmOCIJobPlainHTTPOnlyForPlainHTTPRepos(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		ref       string
		plainHTTP bool
		secret    bool
	}{
		{"oci://registry.lab:5000/charts/app", true, false},
		{"oci://ghcr.io/org/nginx", false, false},        // anonymous https: no access at all
		{"oci://ghcr.io/org/private/nginx", false, true}, // credentials, https
		{"oci://docker.io/bitnamicharts/nginx", false, false},
	}
	for _, c := range cases {
		k, cs := newFakeKubernetesProvider()
		req := InstallHelmRequest{Namespace: "apps", ReleaseName: "web", OCIRef: c.ref, OCIAccess: resolvedAccess(t, c.ref)}
		if _, err := k.InstallHelmChart(ctx, req); err != nil {
			t.Fatal(err)
		}
		job := helmJobByAction(t, k, "install")
		cmd := job.Spec.Template.Spec.Containers[0].Command
		has := false
		for _, a := range cmd {
			has = has || a == "--plain-http"
		}
		if has != c.plainHTTP {
			t.Errorf("%s: --plain-http=%v (%q)", c.ref, has, cmd)
		}
		secs, _ := cs.CoreV1().Secrets(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		if (len(secs.Items) == 1) != c.secret {
			t.Errorf("%s: %d secrets", c.ref, len(secs.Items))
		}
		if !c.secret && (job.Spec.Template.Spec.Containers[0].Env != nil || job.Spec.Template.Spec.Volumes != nil) {
			t.Errorf("%s: env/volumes added without credentials", c.ref)
		}
		if !c.plainHTTP && !c.secret {
			want := []string{"helm", "install", "web", c.ref, "-n", "apps", "--create-namespace"}
			if strings.Join(cmd, "\x00") != strings.Join(want, "\x00") {
				t.Errorf("anonymous args changed: %q", cmd)
			}
		}
	}
	// An access for another registry than the reference is refused.
	k, _ := newFakeKubernetesProvider()
	a := resolvedAccess(t, "oci://registry.lab:5000/charts/app")
	if _, err := k.InstallHelmChart(ctx, InstallHelmRequest{Namespace: "apps", ReleaseName: "web", OCIRef: "oci://ghcr.io/x/y", OCIAccess: a}); HTTPStatus(err) != 400 {
		t.Fatalf("mismatched access: %v", err)
	}
	// Index installs ignore access entirely and stay byte-for-byte the same.
	k, _ = newFakeKubernetesProvider()
	if _, err := k.InstallHelmChart(ctx, InstallHelmRequest{Namespace: "apps", ReleaseName: "web", RepoURL: "https://charts.example.com", ChartName: "nginx"}); err != nil {
		t.Fatal(err)
	}
	if got := helmJobByAction(t, k, "install").Spec.Template.Spec.Containers[0].Command; strings.Join(got, " ") != "helm install web nginx --repo https://charts.example.com -n apps --create-namespace" {
		t.Fatalf("index args: %q", got)
	}
}
