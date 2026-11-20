package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// demoForeignManagers simulates objects another tool (Argo CD) also manages, so the
// "forced apply takes field ownership" warning can be exercised without a cluster.
var demoForeignManagers = map[string][]string{
	key("payments", "Deployment", "payments-api"): {"argocd-application-controller"},
}

// DiffManifest previews an apply in demo mode: the proposed object is the manifest as written
// (JSON is re-indented; YAML is shown as typed), the live object is the demo provider's own
// manifest for that resource. It never changes anything.
func (d *DemoProvider) DiffManifest(ctx context.Context, manifest string) (DiffResult, error) {
	manifest = strings.TrimSpace(manifest)
	if manifest == "" {
		return DiffResult{Valid: false, Errors: []string{"empty manifest"}}, nil
	}
	if hasMultipleDocuments(manifest) {
		return DiffResult{Valid: false, Errors: []string{errMultiDocument}}, nil
	}
	kind, apiVersion, name, ns := parseManifestFields(manifest)
	if kind == "" || apiVersion == "" {
		return DiffResult{Valid: false, Errors: []string{"kind and apiVersion are required"}}, nil
	}
	if name == "" {
		return DiffResult{Valid: false, Errors: []string{"metadata.name is required"}}, nil
	}
	if ns == "" {
		ns = "default"
	}

	obj := DiffObject{Kind: kind, APIVersion: apiVersion, Namespace: ns, Name: name, Proposed: manifest}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(manifest), &parsed); err == nil {
		stripServerFields(parsed)
		maskSecretValues(parsed)
		obj.Proposed = renderDiffJSON(parsed)
	} else if strings.EqualFold(kind, "Secret") {
		// Demo mode cannot parse YAML to mask Secret values selectively; never echo them.
		obj.Proposed = fmt.Sprintf("# Secret %s/%s — values hidden in the diff preview", ns, name)
	}

	if d.resourceExists(ns, kind, name) {
		if detail, err := d.GetResource(ctx, ns, kind, name); err == nil {
			live := detail.Manifest
			stripServerFields(live)
			obj.Exists = true
			obj.Live = renderDiffJSON(live)
			obj.ForeignManagers = append([]string(nil), demoForeignManagers[key(ns, kind, name)]...)
		}
	}
	obj.Changed = obj.Live != obj.Proposed

	res := DiffResult{Valid: true, Objects: []DiffObject{obj}, Message: "Demo preview of the proposed change; nothing here is sent to a cluster"}
	if w := foreignManagerWarning(kind, name, obj.ForeignManagers); w != "" {
		res.Warnings = append(res.Warnings, w)
	}
	return res, nil
}

// ResourceGraph builds the relationship graph for a demo workload or pod from the provider's
// in-memory objects: owner chain, the Service that selects it, an Ingress of the same name,
// HPAs targeting it and the workload's "<name>-config" ConfigMap when one exists.
func (d *DemoProvider) ResourceGraph(ctx context.Context, ns, kind, name string) (ResourceGraph, error) {
	if !d.resourceExists(ns, kind, name) {
		return ResourceGraph{}, NewStatusError(404, fmt.Errorf("%s %s/%s not found", strings.ToLower(kind), ns, name))
	}
	g := newGraphBuilder()
	isPod := strings.EqualFold(kind, "pod")

	var wl Workload
	var pods []Pod
	d.mu.RLock()
	if isPod {
	outer:
		for _, ps := range d.pods {
			for _, p := range ps {
				if p.Namespace == ns && p.Name == name {
					pods = []Pod{p}
					if w, ok := d.workloads[key(ns, "Deployment", p.WorkloadID)]; ok {
						wl = w
					} else {
						for _, w := range d.workloads {
							if w.Namespace == ns && w.Name == p.WorkloadID {
								wl = w
								break
							}
						}
					}
					break outer
				}
			}
		}
	} else {
		wl = d.workloads[key(ns, kind, name)]
		pods = append(pods, d.pods[ns+"/"+name]...)
	}
	d.mu.RUnlock()

	// canonical kind casing for node IDs
	canon := wl.Kind
	if canon == "" {
		canon = "Deployment"
	}
	var rootID string
	top := "" // node that owns the pods
	if wl.Name != "" {
		top, _ = g.node(canon, ns, wl.Name, wl.Status)
	}
	owner := top
	if strings.EqualFold(canon, "Deployment") && wl.Name != "" {
		rsName := wl.Name + "-7d9f6c5b8"
		rs, _ := g.node("ReplicaSet", ns, rsName, fmt.Sprintf("%d/%d ready", wl.Ready, wl.Desired))
		g.edge(top, rs, "owns")
		owner = rs
	}
	for i, p := range pods {
		if i >= 30 {
			g.truncated = true
			break
		}
		pid, _ := g.node("Pod", ns, p.Name, p.Status)
		if owner != "" {
			g.edge(owner, pid, "owns")
		}
	}
	if isPod {
		rootID = graphNodeID("Pod", ns, name)
	} else {
		rootID = top
	}

	if wl.Name != "" {
		attach := top // services/ingress/hpa attach to the workload even when the root is a pod
		svcs, _ := d.ListServices(ctx, ns)
		for _, s := range svcs {
			if selectorSelectsLabels(s.Selector, map[string]string{"app": wl.Name}) {
				sid, _ := g.node("Service", ns, s.Name, s.Type)
				g.edge(sid, attach, "selects")
				ings, _ := d.ListIngresses(ctx, ns)
				for _, in := range ings {
					if in.Name == s.Name {
						iid, _ := g.node("Ingress", ns, in.Name, strings.Join(in.Hosts, ", "))
						g.edge(iid, sid, "routes")
					}
				}
			}
		}
		hpas, _ := d.ListHPAs(ctx, ns)
		for _, h := range hpas {
			if strings.EqualFold(h.TargetKind, canon) && h.TargetName == wl.Name {
				hid, _ := g.node("HorizontalPodAutoscaler", ns, h.Name, fmt.Sprintf("%d-%d replicas", h.MinReplicas, h.MaxReplicas))
				g.edge(hid, attach, "scales")
			}
		}
		demoConfigMapsMu.Lock()
		_, hasCM := demoConfigMaps[configMapKey(ns, wl.Name+"-config")]
		demoConfigMapsMu.Unlock()
		if hasCM {
			cid, _ := g.node("ConfigMap", ns, wl.Name+"-config", "")
			g.edge(attach, cid, "mounts")
		}
	}
	if rootID == "" || !g.seen[rootID] {
		return ResourceGraph{}, NewStatusError(404, fmt.Errorf("%s %s/%s not found", strings.ToLower(kind), ns, name))
	}
	return g.build(rootID), nil
}
