//go:build kubernetes

package platform

import (
	"sync"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

var (
	podsGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	podKind = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}
)

// newFakeFullProvider wires a *KubernetesProvider to BOTH a fake clientset
// (which also backs discovery) and a fake dynamic client seeded with objects.
// When gvrToListKind is nil, CRDs that ClusterHealth LISTs (cert-manager
// Certificates) are still registered so the fake does not panic the way a
// real apiserver would return NotFound for a missing CRD.
func newFakeFullProvider(gvrToListKind map[schema.GroupVersionResource]string, dynObjects []runtime.Object, objects ...runtime.Object) (*KubernetesProvider, *k8sfake.Clientset, *dynamicfake.FakeDynamicClient) {
	cs := k8sfake.NewSimpleClientset(objects...)
	if gvrToListKind == nil {
		gvrToListKind = map[schema.GroupVersionResource]string{}
	}
	if _, ok := gvrToListKind[certManagerCertificateGVR]; !ok {
		gvrToListKind[certManagerCertificateGVR] = "CertificateList"
	}
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind, dynObjects...)
	return &KubernetesProvider{client: cs, dynamic: dyn}, cs, dyn
}

// evictionRecorder stands in for the API server's eviction subresource, which
// client-go's fake clientset does NOT implement (Evict is recorded as a no-op
// "create pods/eviction" action and the pod is left untouched). Installing it
// makes EvictPod actually remove the pod, refuse it (HTTP 429, like a
// PodDisruptionBudget) for names registered via block, and 404 for missing pods.
type evictionRecorder struct {
	mu      sync.Mutex
	evicted []string
	blocked map[string]bool
}

func installEvictionReactor(cs *k8sfake.Clientset, blocked ...string) *evictionRecorder {
	rec := &evictionRecorder{blocked: map[string]bool{}}
	for _, b := range blocked {
		rec.blocked[b] = true
	}
	cs.PrependReactor("create", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "eviction" {
			return false, nil, nil
		}
		ev, ok := action.(clienttesting.CreateAction).GetObject().(*policyv1.Eviction)
		if !ok {
			return false, nil, nil
		}
		key := action.GetNamespace() + "/" + ev.Name
		rec.mu.Lock()
		defer rec.mu.Unlock()
		if rec.blocked[key] {
			return true, nil, apierrors.NewTooManyRequests("Cannot evict pod as it would violate the pod's disruption budget.", 0)
		}
		if _, err := cs.Tracker().Get(podsGVR, action.GetNamespace(), ev.Name); err != nil {
			return true, nil, err
		}
		if err := cs.Tracker().Delete(podsGVR, action.GetNamespace(), ev.Name); err != nil {
			return true, nil, err
		}
		rec.evicted = append(rec.evicted, key)
		return true, nil, nil
	})
	return rec
}

// installNodeNameFieldSelector makes the fake honour the `spec.nodeName=<node>`
// field selector on pod lists (the fake ignores field selectors entirely, so
// Drain would otherwise see every pod in the cluster).
func installNodeNameFieldSelector(cs *k8sfake.Clientset) {
	cs.PrependReactor("list", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		la := action.(clienttesting.ListAction)
		node, ok := la.GetListRestrictions().Fields.RequiresExactMatch("spec.nodeName")
		if !ok {
			return false, nil, nil
		}
		obj, err := cs.Tracker().List(podsGVR, podKind, la.GetNamespace())
		if err != nil {
			return true, nil, err
		}
		all := obj.(*corev1.PodList)
		out := &corev1.PodList{}
		for _, p := range all.Items {
			if p.Spec.NodeName == node {
				out.Items = append(out.Items, p)
			}
		}
		return true, out, nil
	})
}

// installJobNameGenerator emulates the API server's metadata.generateName
// handling for Jobs (the fake stores such objects with an empty name).
func installJobNameGenerator(cs *k8sfake.Clientset) {
	cs.PrependReactor("create", "jobs", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if job, ok := action.(clienttesting.CreateAction).GetObject().(*batchv1.Job); ok && job.Name == "" && job.GenerateName != "" {
			job.Name = job.GenerateName + "x7k2p"
		}
		return false, nil, nil
	})
}

func wantStatus(t *testing.T, err error, want int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error mapping to HTTP %d, got nil", want)
	}
	if got := HTTPStatus(err); got != want {
		t.Fatalf("HTTPStatus(%v) = %d, want %d", err, got, want)
	}
}

func i32(v int32) *int32 { return &v }

func newUnstructured(apiVersion, kind, ns, name string, fields map[string]any) *unstructured.Unstructured {
	obj := map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]any{"name": name},
	}
	if ns != "" {
		obj["metadata"].(map[string]any)["namespace"] = ns
	}
	for k, v := range fields {
		obj[k] = v
	}
	return &unstructured.Unstructured{Object: obj}
}
