//go:build kubernetes

package platform

import (
	"context"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/metadata"
)

var secretMetaGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}

// listSecretsMetadata lists Secret names/ages without pulling .data.
// Type/KeyCount are filled on GetSecret; PartialObjectMetadata has no Type.
func (k *KubernetesProvider) listSecretsMetadata(ctx context.Context, ns string) ([]SecretSummary, bool) {
	if k == nil || k.config == nil {
		return nil, false
	}
	mc, err := metadata.NewForConfig(k.config)
	if err != nil {
		return nil, false
	}
	const page = int64(500)
	var out []SecretSummary
	var cont string
	for {
		list, err := mc.Resource(secretMetaGVR).Namespace(nsSelector(ns)).List(ctx, metav1.ListOptions{Limit: page, Continue: cont})
		if err != nil {
			return nil, false
		}
		for _, s := range list.Items {
			out = append(out, SecretSummary{
				Namespace: s.GetNamespace(), Name: s.GetName(),
				Type: "", Keys: nil, KeyCount: 0,
				Age: age(s.GetCreationTimestamp().Time),
			})
		}
		cont = list.GetContinue()
		if cont == "" {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace == out[j].Namespace {
			return out[i].Name < out[j].Name
		}
		return out[i].Namespace < out[j].Namespace
	})
	return out, true
}
