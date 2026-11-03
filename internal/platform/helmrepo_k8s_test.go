//go:build kubernetes

package platform

import (
	"context"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestK8sHelmRepoStore(t *testing.T) {
	_, cs := newFakeKubernetesProvider()
	s := NewK8sHelmRepoStore(cs, "axiom-system")
	testHelmRepoStore(t, s)

	cm, err := cs.CoreV1().ConfigMaps("axiom-system").Get(context.Background(), helmRepoConfigMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("configmap not written: %v", err)
	}
	var stored []HelmRepo
	if err := json.Unmarshal([]byte(cm.Data[helmRepoConfigMapKey]), &stored); err != nil {
		t.Fatalf("repos.json: %v", err)
	}
	if len(stored) != helmMaxRepos || stored[0].Name != "b" || cm.Labels["app"] != "zyvor-axiom" {
		t.Fatalf("stored: %d repos, first %+v, labels %v", len(stored), stored[0], cm.Labels)
	}

	// A second store over the same cluster (another replica, or a restart) sees the list.
	again, err := NewK8sHelmRepoStore(cs, "axiom-system").List(context.Background())
	if err != nil || len(again) != helmMaxRepos {
		t.Fatalf("reload: %d %v", len(again), err)
	}
}

func TestK8sHelmRepoStoreNamespaceAndExistingConfigMap(t *testing.T) {
	t.Setenv("POD_NAMESPACE", "")
	_, cs := newFakeKubernetesProvider(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: helmRepoConfigMapName, Namespace: "axiom-system"},
		Data:       map[string]string{"other": "kept"},
	})
	s := NewK8sHelmRepoStore(cs, "")
	if s.ns != "axiom-system" {
		t.Fatalf("default namespace: %q", s.ns)
	}
	if l, err := s.List(context.Background()); err != nil || len(l) != 0 {
		t.Fatalf("list: %v %v", l, err)
	}
	if err := s.Add(context.Background(), HelmRepo{Name: "a", URL: "https://a.example.com"}); err != nil {
		t.Fatal(err)
	}
	cm, _ := cs.CoreV1().ConfigMaps("axiom-system").Get(context.Background(), helmRepoConfigMapName, metav1.GetOptions{})
	if cm.Data["other"] != "kept" || cm.Data[helmRepoConfigMapKey] == "" {
		t.Fatalf("data: %v", cm.Data)
	}

	t.Setenv("POD_NAMESPACE", "platform")
	if s := NewK8sHelmRepoStore(cs, ""); s.ns != "platform" {
		t.Fatalf("POD_NAMESPACE: %q", s.ns)
	}

	// Corrupt JSON is an error, not a silently empty list that the next write would clobber.
	cm.Data[helmRepoConfigMapKey] = "{nope"
	if _, err := cs.CoreV1().ConfigMaps("axiom-system").Update(context.Background(), cm, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(context.Background()); err == nil {
		t.Fatal("corrupt repos.json must be reported")
	}
	if err := s.Add(context.Background(), HelmRepo{Name: "b", URL: "https://b.example.com"}); err == nil {
		t.Fatal("must not overwrite a corrupt list")
	}
}

func TestOpenHelmRepoStoreModes(t *testing.T) {
	if _, ok := OpenHelmRepoStore("demo", nil).(*MemoryHelmRepoStore); !ok {
		t.Fatal("demo mode keeps repos in memory")
	}
	p, _ := newFakeKubernetesProvider()
	if _, ok := OpenHelmRepoStore("kubernetes", p).(*K8sHelmRepoStore); !ok {
		t.Fatal("kubernetes mode uses the ConfigMap store")
	}
}
