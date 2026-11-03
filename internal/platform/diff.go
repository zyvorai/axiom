package platform

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// This file holds the provider-independent parts of the manifest diff and the resource graph:
// nothing here imports Kubernetes client libraries, so demo builds keep linking without them.

// errMultiDocument is returned for a manifest holding several YAML documents. Validate, apply
// and diff handle exactly one object; parsing "a --- b" as one document used to apply only the
// first and report success, silently dropping the rest.
const errMultiDocument = "multiple YAML documents are not supported: apply one object at a time"

var yamlDocSeparator = regexp.MustCompile(`(?m)^---[ \t]*$`)

// hasMultipleDocuments reports whether manifest has more than one non-empty YAML document.
func hasMultipleDocuments(manifest string) bool {
	docs := 0
	for _, part := range yamlDocSeparator.Split(manifest, -1) {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		onlyComments := true
		for _, line := range strings.Split(trimmed, "\n") {
			if l := strings.TrimSpace(line); l != "" && !strings.HasPrefix(l, "#") {
				onlyComments = false
				break
			}
		}
		if !onlyComments {
			docs++
		}
	}
	return docs > 1
}

// axiomFieldManager is the server-side-apply field manager Axiom writes with.
const axiomFieldManager = "zyvor-axiom"

// benignFieldManagers are cluster components that own fields on almost every object
// (controllers writing status, the kubelet, ...). Their ownership is not a conflict with a
// GitOps/Helm tool, so they never trigger the "another tool manages this" warning.
var benignFieldManagers = map[string]bool{
	axiomFieldManager:          true,
	"kube-controller-manager":  true,
	"kube-apiserver":           true,
	"kube-scheduler":           true,
	"kubelet":                  true,
	"cloud-controller-manager": true,
	"k3s":                      true,
	"csi-resizer":              true,
	"before-first-apply":       true, // legacy marker the API server adds to pre-SSA objects
}

// isForeignFieldManager reports whether a managedFields manager is something other than Axiom
// or a core cluster component, i.e. a tool (Argo CD, Flux, Helm, kubectl) whose desired state a
// forced server-side apply can take fields away from.
func isForeignFieldManager(manager string) bool {
	return manager != "" && !benignFieldManagers[manager]
}

// foreignManagerWarning is the user-facing warning for a forced server-side apply over fields
// that other managers own. Empty when there are none.
func foreignManagerWarning(kind, name string, managers []string) string {
	if len(managers) == 0 {
		return ""
	}
	return fmt.Sprintf("%s %s is also managed by %s. Axiom applies with force, so conflicting fields move to Axiom and the next sync by that tool may revert or fight this change.",
		kind, name, strings.Join(managers, ", "))
}

// stripServerFields removes server-populated noise from an object map so a live/proposed pair
// diffs only on what a user can actually change.
func stripServerFields(obj map[string]any) {
	delete(obj, "status")
	meta, ok := obj["metadata"].(map[string]any)
	if !ok {
		return
	}
	for _, k := range []string{"managedFields", "resourceVersion", "uid", "generation", "creationTimestamp", "selfLink", "deletionTimestamp", "deletionGracePeriodSeconds"} {
		delete(meta, k)
	}
	if ann, ok := meta["annotations"].(map[string]any); ok {
		// kubectl's last-applied annotation embeds a full copy of the object (Secret data included).
		delete(ann, "kubectl.kubernetes.io/last-applied-configuration")
		if len(ann) == 0 {
			delete(meta, "annotations")
		}
	}
}

const maskedSecretValue = "<redacted>"

// maskSecretValues replaces every value under data/stringData of a Secret with a fixed marker,
// keeping the keys so key additions/removals still show up in the diff. Equal values and
// changed values are therefore indistinguishable — deliberate: a diff must never print a
// secret, and Secret values are only ever shown through the audited secrets endpoints.
func maskSecretValues(obj map[string]any) {
	if kind, _ := obj["kind"].(string); !strings.EqualFold(kind, "Secret") {
		return
	}
	for _, field := range []string{"data", "stringData"} {
		if m, ok := obj[field].(map[string]any); ok {
			for k := range m {
				m[k] = maskedSecretValue
			}
		}
	}
}

// renderDiffJSON renders an object map as stable, indented JSON (map keys are sorted).
func renderDiffJSON(obj map[string]any) string {
	raw, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return ""
	}
	return string(raw)
}

// graphBuilder accumulates nodes/edges, de-duplicating by ID and capping the node count so a
// huge workload cannot produce an unbounded response.
type graphBuilder struct {
	nodes     []GraphNode
	edges     []GraphEdge
	seen      map[string]bool
	edgeSeen  map[string]bool
	truncated bool
	max       int
}

const maxGraphNodes = 120

func newGraphBuilder() *graphBuilder {
	return &graphBuilder{seen: map[string]bool{}, edgeSeen: map[string]bool{}, max: maxGraphNodes}
}

func graphNodeID(kind, ns, name string) string {
	return strings.ToLower(kind) + "/" + ns + "/" + name
}

// node adds (or returns the existing) node and reports whether it is present in the graph.
func (g *graphBuilder) node(kind, ns, name, status string) (string, bool) {
	id := graphNodeID(kind, ns, name)
	if g.seen[id] {
		return id, true
	}
	if len(g.nodes) >= g.max {
		g.truncated = true
		return id, false
	}
	g.seen[id] = true
	g.nodes = append(g.nodes, GraphNode{ID: id, Kind: kind, Namespace: ns, Name: name, Status: status})
	return id, true
}

func (g *graphBuilder) edge(from, to, relation string) {
	if !g.seen[from] || !g.seen[to] {
		return
	}
	k := from + "|" + to + "|" + relation
	if g.edgeSeen[k] {
		return
	}
	g.edgeSeen[k] = true
	g.edges = append(g.edges, GraphEdge{From: from, To: to, Relation: relation})
}

func (g *graphBuilder) build(root string) ResourceGraph {
	sort.SliceStable(g.edges, func(i, j int) bool {
		if g.edges[i].From != g.edges[j].From {
			return g.edges[i].From < g.edges[j].From
		}
		return g.edges[i].To < g.edges[j].To
	})
	return ResourceGraph{Root: root, Nodes: g.nodes, Edges: g.edges, Truncated: g.truncated}
}

// selectorSelectsLabels reports whether every selector pair is present in labels. An empty selector
// matches nothing (a Service without a selector selects no pods).
func selectorSelectsLabels(selector, labels map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}
