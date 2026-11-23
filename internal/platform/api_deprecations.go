package platform

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"

	"sigs.k8s.io/yaml"
)

// Deprecated and removed Kubernetes API check (POST /api/v1/manifests/deprecations).
// It reads manifests the caller sends (nothing is applied and no cluster is
// contacted) and flags apiVersion/kind pairs that Kubernetes has removed or
// deprecated, with the release that removed them and the replacement. The
// table is static and curated from the upstream deprecation guide; it does
// not know about CRDs, so a CRD's own version policy is out of scope.

const (
	// APIDeprecationMaxBytes caps the manifest text accepted.
	APIDeprecationMaxBytes = 1 << 20
	apiDeprecationMaxDocs  = 500
	apiDeprecationMaxItems = 200 // items inside one kind: List
	apiDeprecationMaxHits  = 500
)

var ErrAPIDeprecationInput = errors.New("manifest input is invalid")

type apiDeprecation struct {
	APIVersion   string
	Kind         string // "" matches every kind in the group version
	RemovedIn    string // "" = deprecated but not removed
	DeprecatedIn string
	Replacement  string
}

// apiDeprecationTable is ordered by group for readability only.
var apiDeprecationTable = []apiDeprecation{
	{"extensions/v1beta1", "Ingress", "1.22", "1.14", "networking.k8s.io/v1 Ingress"},
	{"networking.k8s.io/v1beta1", "Ingress", "1.22", "1.19", "networking.k8s.io/v1 Ingress"},
	{"networking.k8s.io/v1beta1", "IngressClass", "1.22", "1.19", "networking.k8s.io/v1 IngressClass"},
	{"extensions/v1beta1", "Deployment", "1.16", "1.9", "apps/v1 Deployment"},
	{"extensions/v1beta1", "DaemonSet", "1.16", "1.9", "apps/v1 DaemonSet"},
	{"extensions/v1beta1", "ReplicaSet", "1.16", "1.9", "apps/v1 ReplicaSet"},
	{"extensions/v1beta1", "NetworkPolicy", "1.16", "1.9", "networking.k8s.io/v1 NetworkPolicy"},
	{"extensions/v1beta1", "PodSecurityPolicy", "1.16", "1.10", "none: PodSecurityPolicy was removed in 1.25, use Pod Security Admission"},
	{"apps/v1beta1", "", "1.16", "1.9", "apps/v1"},
	{"apps/v1beta2", "", "1.16", "1.9", "apps/v1"},
	{"policy/v1beta1", "PodDisruptionBudget", "1.25", "1.21", "policy/v1 PodDisruptionBudget"},
	{"policy/v1beta1", "PodSecurityPolicy", "1.25", "1.21", "none: use Pod Security Admission"},
	{"batch/v1beta1", "CronJob", "1.25", "1.21", "batch/v1 CronJob"},
	{"autoscaling/v2beta1", "HorizontalPodAutoscaler", "1.25", "1.22", "autoscaling/v2 HorizontalPodAutoscaler"},
	{"autoscaling/v2beta2", "HorizontalPodAutoscaler", "1.26", "1.23", "autoscaling/v2 HorizontalPodAutoscaler"},
	{"discovery.k8s.io/v1beta1", "EndpointSlice", "1.25", "1.21", "discovery.k8s.io/v1 EndpointSlice"},
	{"events.k8s.io/v1beta1", "Event", "1.25", "1.19", "events.k8s.io/v1 Event"},
	{"node.k8s.io/v1beta1", "RuntimeClass", "1.25", "1.20", "node.k8s.io/v1 RuntimeClass"},
	{"flowcontrol.apiserver.k8s.io/v1beta1", "", "1.26", "1.23", "flowcontrol.apiserver.k8s.io/v1"},
	{"flowcontrol.apiserver.k8s.io/v1beta2", "", "1.29", "1.26", "flowcontrol.apiserver.k8s.io/v1"},
	{"flowcontrol.apiserver.k8s.io/v1beta3", "", "1.32", "1.29", "flowcontrol.apiserver.k8s.io/v1"},
	{"storage.k8s.io/v1beta1", "CSIStorageCapacity", "1.27", "1.24", "storage.k8s.io/v1 CSIStorageCapacity"},
	{"storage.k8s.io/v1beta1", "", "1.22", "1.19", "storage.k8s.io/v1"},
	{"rbac.authorization.k8s.io/v1beta1", "", "1.22", "1.17", "rbac.authorization.k8s.io/v1"},
	{"rbac.authorization.k8s.io/v1alpha1", "", "1.22", "1.17", "rbac.authorization.k8s.io/v1"},
	{"admissionregistration.k8s.io/v1beta1", "", "1.22", "1.16", "admissionregistration.k8s.io/v1"},
	{"apiextensions.k8s.io/v1beta1", "CustomResourceDefinition", "1.22", "1.16", "apiextensions.k8s.io/v1 CustomResourceDefinition"},
	{"apiregistration.k8s.io/v1beta1", "APIService", "1.22", "1.19", "apiregistration.k8s.io/v1 APIService"},
	{"certificates.k8s.io/v1beta1", "CertificateSigningRequest", "1.22", "1.19", "certificates.k8s.io/v1 CertificateSigningRequest"},
	{"coordination.k8s.io/v1beta1", "Lease", "1.22", "1.19", "coordination.k8s.io/v1 Lease"},
	{"scheduling.k8s.io/v1beta1", "PriorityClass", "1.22", "1.14", "scheduling.k8s.io/v1 PriorityClass"},
	{"authentication.k8s.io/v1beta1", "", "1.22", "1.19", "authentication.k8s.io/v1"},
	{"authorization.k8s.io/v1beta1", "", "1.22", "1.19", "authorization.k8s.io/v1"},
	// Deprecated, still served: v1 Endpoints is replaced by EndpointSlice (1.33).
	{"v1", "Endpoints", "", "1.33", "discovery.k8s.io/v1 EndpointSlice"},
}

// APIDeprecationFinding is one affected object.
type APIDeprecationFinding struct {
	Document     int    `json:"document"`
	APIVersion   string `json:"apiVersion"`
	Kind         string `json:"kind"`
	Name         string `json:"name,omitempty"`
	Namespace    string `json:"namespace,omitempty"`
	Status       string `json:"status"` // removed | deprecated
	DeprecatedIn string `json:"deprecatedIn,omitempty"`
	RemovedIn    string `json:"removedIn,omitempty"`
	Replacement  string `json:"replacement"`
	Message      string `json:"message"`
}

type APIDeprecationReport struct {
	Documents  int                     `json:"documents"`
	Objects    int                     `json:"objects"`
	Removed    int                     `json:"removed"`
	Deprecated int                     `json:"deprecated"`
	Findings   []APIDeprecationFinding `json:"findings"`
	Skipped    []string                `json:"skipped,omitempty"`
	Truncated  bool                    `json:"truncated,omitempty"`
	Note       string                  `json:"note"`
}

func lookupAPIDeprecation(apiVersion, kind string) (apiDeprecation, bool) {
	for _, d := range apiDeprecationTable {
		if d.APIVersion == apiVersion && (d.Kind == "" || d.Kind == kind) {
			return d, true
		}
	}
	return apiDeprecation{}, false
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// CheckAPIDeprecations is pure. It rejects input above APIDeprecationMaxBytes
// or with more than 500 documents; a document that is not a mapping or is
// malformed is listed under Skipped, never fatal, so one bad file does not
// hide the rest.
func CheckAPIDeprecations(manifest string) (APIDeprecationReport, error) {
	if len(manifest) > APIDeprecationMaxBytes {
		return APIDeprecationReport{}, fmt.Errorf("%w: larger than %d bytes", ErrAPIDeprecationInput, APIDeprecationMaxBytes)
	}
	if strings.IndexByte(manifest, 0) >= 0 {
		return APIDeprecationReport{}, fmt.Errorf("%w: contains a NUL byte", ErrAPIDeprecationInput)
	}
	if strings.TrimSpace(manifest) == "" {
		return APIDeprecationReport{}, fmt.Errorf("%w: empty", ErrAPIDeprecationInput)
	}
	docs := splitYAMLDocs(manifest)
	if len(docs) > apiDeprecationMaxDocs {
		return APIDeprecationReport{}, fmt.Errorf("%w: more than %d documents", ErrAPIDeprecationInput, apiDeprecationMaxDocs)
	}
	rep := APIDeprecationReport{Documents: len(docs), Findings: []APIDeprecationFinding{}}
	visit := func(idx int, obj map[string]any) {
		rep.Objects++
		apiVersion, _ := obj["apiVersion"].(string)
		kind, _ := obj["kind"].(string)
		if apiVersion == "" || kind == "" {
			rep.Skipped = append(rep.Skipped, fmt.Sprintf("document %d: no apiVersion/kind", idx))
			return
		}
		d, ok := lookupAPIDeprecation(apiVersion, kind)
		if !ok {
			return
		}
		f := APIDeprecationFinding{Document: idx, APIVersion: clip(apiVersion, 100), Kind: clip(kind, 100),
			DeprecatedIn: d.DeprecatedIn, RemovedIn: d.RemovedIn, Replacement: d.Replacement}
		if md, ok := obj["metadata"].(map[string]any); ok {
			n, _ := md["name"].(string)
			ns, _ := md["namespace"].(string)
			f.Name, f.Namespace = clip(n, 253), clip(ns, 63)
		}
		if d.RemovedIn != "" {
			f.Status = "removed"
			f.Message = fmt.Sprintf("%s %s was removed in Kubernetes %s; use %s.", apiVersion, kind, d.RemovedIn, d.Replacement)
			rep.Removed++
		} else {
			f.Status = "deprecated"
			f.Message = fmt.Sprintf("%s %s is deprecated since Kubernetes %s and still served; move to %s.", apiVersion, kind, d.DeprecatedIn, d.Replacement)
			rep.Deprecated++
		}
		if len(rep.Findings) >= apiDeprecationMaxHits {
			rep.Truncated = true
			return
		}
		rep.Findings = append(rep.Findings, f)
	}
	for i, doc := range docs {
		idx := i + 1
		if strings.TrimSpace(doc) == "" {
			continue
		}
		var obj map[string]any
		err := yaml.Unmarshal([]byte(doc), &obj)
		if err == nil && obj == nil && !strings.Contains(doc, ":") {
			continue // comments only
		}
		if err != nil || obj == nil {
			rep.Skipped = append(rep.Skipped, fmt.Sprintf("document %d: not a YAML mapping", idx))
			continue
		}
		if k, _ := obj["kind"].(string); strings.HasSuffix(k, "List") {
			if items, ok := obj["items"].([]any); ok {
				for n, it := range items {
					if n >= apiDeprecationMaxItems {
						rep.Skipped = append(rep.Skipped, fmt.Sprintf("document %d: only the first %d list items were checked", idx, apiDeprecationMaxItems))
						break
					}
					if m, ok := it.(map[string]any); ok {
						visit(idx, m)
					}
				}
				continue
			}
		}
		visit(idx, obj)
	}
	if len(rep.Skipped) > 50 {
		rep.Skipped = append(rep.Skipped[:50], "more skipped documents not listed")
	}
	sort.SliceStable(rep.Findings, func(i, j int) bool {
		if rep.Findings[i].Status != rep.Findings[j].Status {
			return rep.Findings[i].Status == "removed"
		}
		return rep.Findings[i].Document < rep.Findings[j].Document
	})
	rep.Note = "Static table from the Kubernetes deprecation guide; it knows built-in APIs only, not CRDs. Nothing is applied and no cluster is contacted. A clean result does not prove the manifests are valid."
	return rep, nil
}

// splitYAMLDocs splits on lines that are exactly "---" (optionally followed
// by a comment or spaces).
func splitYAMLDocs(s string) []string {
	var docs []string
	var cur bytes.Buffer
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		t := strings.TrimRight(line, " \t")
		if t == "---" || strings.HasPrefix(t, "--- ") || strings.HasPrefix(t, "---#") {
			docs = append(docs, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteString(line)
		cur.WriteByte('\n')
	}
	docs = append(docs, cur.String())
	// drop a leading empty document produced by a manifest starting with ---
	var out []string
	for _, d := range docs {
		if strings.TrimSpace(d) != "" {
			out = append(out, d)
		}
	}
	return out
}
