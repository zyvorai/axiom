//go:build kubernetes

package platform

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/yaml"
)

// DiffManifest previews ApplyManifest without changing anything. It issues the same forced
// server-side apply as ApplyManifest but with dryRun=All, so the API server runs defaulting,
// validation and admission and returns the object as it WOULD be stored. That is compared with
// the live object (both stripped of server-managed fields, Secret values masked), and the live
// object's managedFields are inspected for other tools (Argo CD, Flux, Helm, kubectl) whose
// fields a forced apply would take over.
func (k *KubernetesProvider) DiffManifest(ctx context.Context, manifest string) (DiffResult, error) {
	manifest = strings.TrimSpace(manifest)
	if manifest == "" {
		return DiffResult{Valid: false, Errors: []string{"empty manifest"}}, nil
	}
	if hasMultipleDocuments(manifest) {
		return DiffResult{Valid: false, Errors: []string{errMultiDocument}}, nil
	}
	var obj unstructured.Unstructured
	if err := json.Unmarshal([]byte(manifest), &obj.Object); err != nil {
		obj.Object = nil
		if err := yaml.Unmarshal([]byte(manifest), &obj.Object); err != nil {
			return DiffResult{Valid: false, Errors: []string{err.Error()}}, nil
		}
	}
	gvk := obj.GroupVersionKind()
	if gvk.Kind == "" || gvk.Version == "" {
		return DiffResult{Valid: false, Errors: []string{"kind and apiVersion are required"}}, nil
	}
	// A manifest copied from the live object (the YAML tabs) carries managedFields, which the API
	// server rejects in an apply request; they are bookkeeping, not desired state.
	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")
	name, ns := obj.GetName(), obj.GetNamespace()
	if name == "" {
		return DiffResult{Valid: false, Errors: []string{"metadata.name is required"}}, nil
	}
	if k.dynamic == nil || k.client == nil {
		return DiffResult{Valid: false, Errors: []string{"dynamic client unavailable"}}, nil
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(k.client.Discovery()))
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return DiffResult{Valid: false, Errors: []string{err.Error()}}, nil
	}
	namespaced := mapping.Scope.Name() == "namespace"
	if namespaced && ns == "" {
		ns = "default"
	}
	if !namespaced {
		ns = ""
	}
	raw, err := json.Marshal(obj.Object)
	if err != nil {
		return DiffResult{Valid: false, Errors: []string{err.Error()}}, nil
	}

	var res dynamicResource
	if namespaced {
		res = k.dynamic.Resource(mapping.Resource).Namespace(ns)
	} else {
		res = k.dynamic.Resource(mapping.Resource)
	}

	out := DiffObject{Kind: gvk.Kind, APIVersion: gvk.GroupVersion().String(), Namespace: ns, Name: name}

	var live map[string]any
	var managers []string
	existing, getErr := res.Get(ctx, name, metav1.GetOptions{})
	switch {
	case getErr == nil:
		out.Exists = true
		managers = foreignManagersOf(existing.GetManagedFields())
		live = existing.Object
	case apierrors.IsNotFound(getErr):
		// a new object: everything is an addition
	default:
		return DiffResult{Valid: false, Errors: []string{getErr.Error()}}, nil
	}

	force := true
	dry, applyErr := res.Patch(ctx, name, types.ApplyPatchType, raw, metav1.PatchOptions{
		FieldManager: axiomFieldManager, Force: &force, DryRun: []string{metav1.DryRunAll},
	})
	if applyErr != nil {
		return DiffResult{Valid: false, Errors: []string{applyErr.Error()}}, nil
	}

	proposed := dry.Object
	for _, m := range []map[string]any{live, proposed} {
		if m != nil {
			stripServerFields(m)
			maskSecretValues(m)
		}
	}
	if live != nil {
		b, _ := yaml.Marshal(live)
		out.Live = string(b)
	}
	pb, _ := yaml.Marshal(proposed)
	out.Proposed = string(pb)
	out.Changed = out.Live != out.Proposed
	out.ForeignManagers = managers

	result := DiffResult{Valid: true, Objects: []DiffObject{out}}
	if w := foreignManagerWarning(gvk.Kind, name, managers); w != "" {
		result.Warnings = append(result.Warnings, w)
	}
	return result, nil
}

// dynamicResource is the subset of dynamic.ResourceInterface DiffManifest needs.
type dynamicResource interface {
	Get(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, options metav1.PatchOptions, subresources ...string) (*unstructured.Unstructured, error)
}

// foreignManagersOf returns the distinct field managers (other than Axiom and core cluster
// components) that own non-status fields of an object, sorted.
func foreignManagersOf(entries []metav1.ManagedFieldsEntry) []string {
	seen := map[string]bool{}
	for _, e := range entries {
		if e.Subresource != "" || !isForeignFieldManager(e.Manager) {
			continue
		}
		seen[e.Manager] = true
	}
	out := make([]string, 0, len(seen))
	for m := range seen {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}
