//go:build kubernetes

package platform

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHelmOCIInstallJobArgs(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	cmd := func(action string) []string {
		t.Helper()
		jobs, _ := cs.BatchV1().Jobs(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		for _, j := range jobs.Items {
			if j.Labels["axiom.zyvor.dev/helm-action"] == action {
				return j.Spec.Template.Spec.Containers[0].Command
			}
		}
		t.Fatalf("no %s job", action)
		return nil
	}

	ref := "oci://ghcr.io/org/charts/nginx"
	if _, err := k.InstallHelmChart(ctx, InstallHelmRequest{Namespace: "apps", ReleaseName: "web", OCIRef: ref, ChartVersion: "1.2.0"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"helm", "install", "web", ref, "-n", "apps", "--create-namespace", "--version", "1.2.0"}
	if got := cmd("install"); strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("install args:\n got %q\nwant %q", got, want)
	}
	if _, err := k.UpgradeHelmChart(ctx, InstallHelmRequest{Namespace: "apps", ReleaseName: "web", OCIRef: ref}); err != nil {
		t.Fatal(err)
	}
	want = []string{"helm", "upgrade", "web", ref, "-n", "apps"}
	if got := cmd("upgrade"); strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("upgrade args:\n got %q\nwant %q", got, want)
	}
	for _, c := range append(cmd("install"), cmd("upgrade")...) {
		if c == "--repo" || c == "sh" || c == "-c" {
			t.Fatalf("OCI job must not use %q", c)
		}
	}
}

func TestHelmIndexInstallJobArgsUnchanged(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	if _, err := k.InstallHelmChart(ctx, InstallHelmRequest{Namespace: "apps", ReleaseName: "web", RepoURL: "https://charts.example.com", ChartName: "nginx", ChartVersion: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	jobs, _ := cs.BatchV1().Jobs(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if len(jobs.Items) != 1 {
		t.Fatalf("jobs: %d", len(jobs.Items))
	}
	got := jobs.Items[0].Spec.Template.Spec.Containers[0].Command
	want := []string{"helm", "install", "web", "nginx", "--repo", "https://charts.example.com", "-n", "apps", "--create-namespace", "--version", "1.0.0"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("index install args changed:\n got %q\nwant %q", got, want)
	}
	if _, err := k.InstallHelmChart(ctx, InstallHelmRequest{Namespace: "apps", ReleaseName: "web2", RepoURL: "https://x"}); err == nil || err.Error() != "namespace, releaseName, repoUrl, and chartName are required" {
		t.Fatalf("index validation message changed: %v", err)
	}
}

func TestHelmOCIInstallRejectsBadRefs(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	for _, ref := range []string{
		"oci://u:p@ghcr.io/org/nginx",
		"oci://ghcr.io/org/nginx --post-renderer=/bin/sh",
		"https://ghcr.io/org/nginx",
		"oci://ghcr.io",
		"-oci://ghcr.io/org/nginx",
	} {
		_, err := k.InstallHelmChart(ctx, InstallHelmRequest{Namespace: "apps", ReleaseName: "web", OCIRef: ref})
		if err == nil || HTTPStatus(err) != 400 {
			t.Errorf("install %q: %v", ref, err)
		}
		if _, err := k.UpgradeHelmChart(ctx, InstallHelmRequest{Namespace: "apps", ReleaseName: "web", OCIRef: ref}); err == nil {
			t.Errorf("upgrade %q accepted", ref)
		}
	}
	if _, err := k.InstallHelmChart(ctx, InstallHelmRequest{Namespace: "apps", ReleaseName: "--dry-run", OCIRef: "oci://ghcr.io/org/nginx"}); err == nil {
		t.Error("flag-like release accepted")
	}
	jobs, _ := cs.BatchV1().Jobs(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if len(jobs.Items) != 0 {
		t.Fatalf("rejected requests created %d jobs", len(jobs.Items))
	}
}
