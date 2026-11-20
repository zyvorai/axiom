package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// validateAgainstVeleroCRD checks that obj's top-level kind/apiVersion and every
// key under .spec (and nested maps we set) exist in the upstream Velero CRD
// OpenAPI schema checked into testdata/velero/. This catches drift when we
// invent a field Velero would reject at admission.
func validateAgainstVeleroCRD(crdFile string, obj *unstructured.Unstructured) error {
	raw, err := os.ReadFile(crdFile)
	if err != nil {
		return err
	}
	var crd map[string]any
	if err := yaml.Unmarshal(raw, &crd); err != nil {
		return fmt.Errorf("parse CRD: %w", err)
	}
	versions, _, _ := unstructured.NestedSlice(crd, "spec", "versions")
	if len(versions) == 0 {
		return fmt.Errorf("CRD has no versions")
	}
	v0, _ := versions[0].(map[string]any)
	schema, _, _ := unstructured.NestedMap(v0, "schema", "openAPIV3Schema")
	if schema == nil {
		return fmt.Errorf("CRD missing openAPIV3Schema")
	}
	wantKind, _, _ := unstructured.NestedString(crd, "spec", "names", "kind")
	if obj.GetKind() != wantKind {
		return fmt.Errorf("kind %q != CRD %q", obj.GetKind(), wantKind)
	}
	if !strings.HasPrefix(obj.GetAPIVersion(), "velero.io/") {
		return fmt.Errorf("apiVersion %q is not velero.io", obj.GetAPIVersion())
	}
	props, _, _ := unstructured.NestedMap(schema, "properties")
	return validateOpenAPIObject(props, obj.Object, "")
}

func validateOpenAPIObject(props map[string]any, obj map[string]any, path string) error {
	if props == nil {
		return nil
	}
	for k, v := range obj {
		if k == "apiVersion" || k == "kind" || k == "status" {
			continue
		}
		schema, ok := props[k].(map[string]any)
		if !ok {
			return fmt.Errorf("%s: unknown field %q (not in Velero CRD)", path+"/"+k, k)
		}
		if k == "metadata" {
			continue // standard ObjectMeta; CRD marks it as type:object only
		}
		switch typed := v.(type) {
		case map[string]any:
			child, _, _ := unstructured.NestedMap(schema, "properties")
			if child != nil {
				if err := validateOpenAPIObject(child, typed, path+"/"+k); err != nil {
					return err
				}
			}
		case []any:
			items, _, _ := unstructured.NestedMap(schema, "items")
			if items == nil {
				continue
			}
			itemProps, _, _ := unstructured.NestedMap(items, "properties")
			for i, el := range typed {
				m, ok := el.(map[string]any)
				if !ok || itemProps == nil {
					continue
				}
				if err := validateOpenAPIObject(itemProps, m, fmt.Sprintf("%s/%s[%d]", path, k, i)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func veleroTestdata(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata", "velero", name)
}
