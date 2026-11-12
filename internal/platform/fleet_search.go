package platform

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Fleet search: "where is payments running?". A query is run against every
// registered cluster through that cluster's own provider (never the session's
// active one), with bounded concurrency and a per-cluster timeout. Clusters the
// prober last saw as unreachable are skipped rather than waited on, and a
// cluster that fails is reported next to the results instead of failing the
// whole search.

// Kinds a fleet search can return.
const (
	FleetSearchWorkload  = "Workload"
	FleetSearchService   = "Service"
	FleetSearchNamespace = "Namespace"
	FleetSearchPod       = "Pod"
)

// FleetSearchKinds are the valid kinds, in their default order.
var FleetSearchKinds = []string{FleetSearchWorkload, FleetSearchService, FleetSearchNamespace, FleetSearchPod}

const (
	// FleetSearchMinQuery and FleetSearchMaxQuery bound the trimmed query length (in characters).
	FleetSearchMinQuery = 2
	FleetSearchMaxQuery = 64
	// FleetSearchDefaultLimit and FleetSearchMaxLimit bound the number of results returned.
	FleetSearchDefaultLimit = 50
	FleetSearchMaxLimit     = 200
)

// fleetSearchTimeout bounds the search of one cluster; a variable so tests can shorten it.
var fleetSearchTimeout = 5 * time.Second

// FleetSearchHit is one matching object in one cluster.
type FleetSearchHit struct {
	Cluster      string `json:"cluster"`
	ClusterLabel string `json:"clusterLabel"`
	ClusterState string `json:"clusterState"`
	Kind         string `json:"kind"`
	WorkloadKind string `json:"workloadKind,omitempty"`
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	Detail       string `json:"detail"`

	rank    int // 0 exact name, 1 name prefix, 2 name contains, 3 namespace/image
	cluster int // registration order
}

// FleetSearchSkip is a cluster that was not searched.
type FleetSearchSkip struct {
	Cluster string `json:"cluster"`
	Reason  string `json:"reason"`
}

// FleetSearchError is a cluster whose search failed.
type FleetSearchError struct {
	Cluster string `json:"cluster"`
	Error   string `json:"error"`
}

// FleetSearchClusters reports which clusters took part.
type FleetSearchClusters struct {
	Searched int                `json:"searched"` // clusters asked (including those that then failed)
	Skipped  []FleetSearchSkip  `json:"skipped"`
	Errors   []FleetSearchError `json:"errors"`
}

// FleetSearchResult is the response of GET /api/v1/fleet/search.
type FleetSearchResult struct {
	Query     string              `json:"query"`
	Results   []FleetSearchHit    `json:"results"`
	Total     int                 `json:"total"`
	Truncated bool                `json:"truncated"`
	Clusters  FleetSearchClusters `json:"clusters"`
}

// EmptyFleetSearch is the answer when there is no fleet to search.
func EmptyFleetSearch(q string) FleetSearchResult {
	return FleetSearchResult{Query: q, Results: []FleetSearchHit{},
		Clusters: FleetSearchClusters{Skipped: []FleetSearchSkip{}, Errors: []FleetSearchError{}}}
}

// ParseFleetSearchKinds validates a comma-separated kind filter (case-insensitive).
// An empty value means every kind.
func ParseFleetSearchKinds(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return append([]string(nil), FleetSearchKinds...), nil
	}
	seen := map[string]bool{}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		var k string
		for _, valid := range FleetSearchKinds {
			if strings.EqualFold(part, valid) {
				k = valid
			}
		}
		if k == "" {
			return nil, fmt.Errorf("kind must be one of %s", strings.Join(FleetSearchKinds, ", "))
		}
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out, nil
}

type fleetSearchTarget struct {
	name, label string
	p           Provider
	order       int
}

func (sp *SwitchableProvider) searchTargets() []fleetSearchTarget {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	out := make([]fleetSearchTarget, 0, len(sp.order))
	for i, n := range sp.order {
		e := sp.entries[n]
		out = append(out, fleetSearchTarget{name: n, label: e.label, p: e.provider, order: i})
	}
	return out
}

// SearchFleet finds workloads, services and namespaces whose name or namespace
// (or, for workloads, image) contains q, across every registered cluster. q is
// matched case-insensitively; kinds empty means all kinds; limit is clamped to
// 1..FleetSearchMaxLimit. Callers validate q (see the API handler).
func (sp *SwitchableProvider) SearchFleet(ctx context.Context, q string, kinds []string, limit int) FleetSearchResult {
	q = strings.TrimSpace(q)
	out := EmptyFleetSearch(q)
	if limit <= 0 {
		limit = FleetSearchDefaultLimit
	}
	if limit > FleetSearchMaxLimit {
		limit = FleetSearchMaxLimit
	}
	if len(kinds) == 0 {
		kinds = FleetSearchKinds
	}
	want := map[string]bool{}
	for _, k := range kinds {
		want[k] = true
	}
	sealedOnly := false
	needle := strings.ToLower(q)
	if strings.HasPrefix(needle, "sealed:") {
		sealedOnly = true
		needle = strings.TrimSpace(strings.TrimPrefix(needle, "sealed:"))
		out.Query = q
	}

	targets := sp.searchTargets()
	// A cluster that has never been probed has no state to go by; probe it
	// first (as Fleet does) so a dead cluster is skipped rather than waited on.
	var unprobed []probeTarget
	for _, t := range targets {
		if sp.healthOf(t.name).State == ClusterUnknown {
			unprobed = append(unprobed, probeTarget{name: t.name, p: t.p})
		}
	}
	if len(unprobed) > 0 {
		sp.probeTargets(ctx, unprobed)
	}

	type outcome struct {
		t    fleetSearchTarget
		hits []FleetSearchHit
		err  error
	}
	var live []fleetSearchTarget
	states := map[string]string{}
	for _, t := range targets {
		st := sp.healthOf(t.name).State
		states[t.name] = st
		if st == ClusterUnreachable {
			out.Clusters.Skipped = append(out.Clusters.Skipped, FleetSearchSkip{Cluster: t.name, Reason: ClusterUnreachable})
			continue
		}
		live = append(live, t)
	}
	out.Clusters.Searched = len(live)

	results := make([]outcome, len(live))
	sem := make(chan struct{}, fleetProbeConcurrency)
	var wg sync.WaitGroup
	for i, t := range live {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, t fleetSearchTarget) {
			defer wg.Done()
			defer func() { <-sem }()
			hits, err := searchCluster(ctx, t, states[t.name], needle, want)
			if err == nil && sealedOnly {
				hits = filterSealedHits(ctx, t.p, hits)
			}
			results[i] = outcome{t: t, hits: hits, err: err}
		}(i, t)
	}
	wg.Wait()

	var all []FleetSearchHit
	for _, o := range results {
		if o.err != nil {
			out.Clusters.Errors = append(out.Clusters.Errors, FleetSearchError{Cluster: o.t.name, Error: fleetSearchError(o.err)})
			continue
		}
		all = append(all, o.hits...)
	}
	sortFleetHits(all)
	out.Total = len(all)
	if len(all) > limit {
		all = all[:limit]
		out.Truncated = true
	}
	out.Results = append(out.Results, all...)
	return out
}

// searchCluster runs the list calls for one cluster under the per-cluster
// timeout. The calls run in their own goroutine so a provider that ignores its
// context still cannot hold the search past the deadline.
func searchCluster(ctx context.Context, t fleetSearchTarget, state, needle string, want map[string]bool) ([]FleetSearchHit, error) {
	if t.p == nil {
		return nil, errors.New("no provider registered")
	}
	cctx, cancel := context.WithTimeout(ctx, fleetSearchTimeout)
	defer cancel()
	type res struct {
		hits []FleetSearchHit
		err  error
	}
	ch := make(chan res, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- res{err: fmt.Errorf("search failed: %v", r)}
			}
		}()
		h, err := listClusterMatches(cctx, t, state, needle, want)
		ch <- res{h, err}
	}()
	select {
	case r := <-ch:
		if r.err == nil && cctx.Err() != nil {
			r.err = cctx.Err()
		}
		if errors.Is(r.err, context.DeadlineExceeded) {
			r.err = fmt.Errorf("timed out after %s", fleetSearchTimeout)
		}
		return r.hits, r.err
	case <-cctx.Done():
		if errors.Is(cctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("timed out after %s", fleetSearchTimeout)
		}
		return nil, cctx.Err()
	}
}

func listClusterMatches(ctx context.Context, t fleetSearchTarget, state, needle string, want map[string]bool) ([]FleetSearchHit, error) {
	var hits []FleetSearchHit
	base := FleetSearchHit{Cluster: t.name, ClusterLabel: t.label, ClusterState: state, cluster: t.order}
	if want[FleetSearchWorkload] {
		wls, err := t.p.ListWorkloads(ctx, "all")
		if err != nil {
			return nil, fmt.Errorf("listing workloads: %w", err)
		}
		for _, w := range wls {
			rank, ok := matchRank(needle, w.Name, w.Namespace, w.Image)
			if !ok {
				continue
			}
			h := base
			h.Kind, h.WorkloadKind, h.Namespace, h.Name, h.Status, h.rank = FleetSearchWorkload, w.Kind, w.Namespace, w.Name, w.Status, rank
			h.Detail = fmt.Sprintf("%d/%d ready", w.Ready, w.Desired)
			if w.Image != "" {
				h.Detail += " · " + w.Image
			}
			hits = append(hits, h)
		}
	}
	if want[FleetSearchService] {
		svcs, err := t.p.ListServices(ctx, "all")
		if err != nil {
			return nil, fmt.Errorf("listing services: %w", err)
		}
		for _, s := range svcs {
			rank, ok := matchRank(needle, s.Name, s.Namespace)
			if !ok {
				continue
			}
			h := base
			h.Kind, h.Namespace, h.Name, h.rank = FleetSearchService, s.Namespace, s.Name, rank
			h.Status = "No endpoints"
			if len(s.Endpoints) > 0 {
				h.Status = fmt.Sprintf("%d endpoints", len(s.Endpoints))
			}
			h.Detail = serviceDetail(s)
			hits = append(hits, h)
		}
	}
	if want[FleetSearchNamespace] {
		nss, err := t.p.ListNamespaces(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing namespaces: %w", err)
		}
		for _, n := range nss {
			rank, ok := matchRank(needle, n.Name)
			if !ok {
				continue
			}
			h := base
			h.Kind, h.Namespace, h.Name, h.Status, h.rank = FleetSearchNamespace, n.Name, n.Name, n.Status, rank
			h.Detail = fmt.Sprintf("%d pods · %d services", n.Pods, n.Services)
			hits = append(hits, h)
		}
	}
	if want[FleetSearchPod] {
		pods, err := t.p.ListPods(ctx, "all")
		if err != nil {
			return nil, fmt.Errorf("listing pods: %w", err)
		}
		for _, pod := range pods {
			labelBlob := ""
			for k, v := range pod.Labels {
				labelBlob += " " + k + "=" + v
			}
			rank, ok := matchRank(needle, pod.Name, pod.Namespace, pod.Image, labelBlob)
			if !ok {
				continue
			}
			h := base
			h.Kind, h.Namespace, h.Name, h.Status, h.rank = FleetSearchPod, pod.Namespace, pod.Name, pod.Status, rank
			h.Detail = fmt.Sprintf("%s · %s", pod.Ready, pod.Node)
			if pod.Image != "" {
				h.Detail += " · " + pod.Image
			}
			hits = append(hits, h)
		}
	}
	return hits, nil
}

func serviceDetail(s Service) string {
	parts := []string{}
	if s.Type != "" {
		parts = append(parts, s.Type)
	}
	if s.ClusterIP != "" {
		parts = append(parts, s.ClusterIP)
	}
	var ports []string
	for _, p := range s.Ports {
		proto := p.Protocol
		if proto == "" {
			proto = "TCP"
		}
		ports = append(ports, fmt.Sprintf("%d/%s", p.Port, proto))
	}
	if len(ports) > 0 {
		parts = append(parts, strings.Join(ports, ", "))
	}
	return strings.Join(parts, " · ")
}

// filterSealedHits keeps workloads/pods that are network-isolated (Seal).
// Empty needle with sealed: matches every sealed object.
func filterSealedHits(ctx context.Context, p Provider, hits []FleetSearchHit) []FleetSearchHit {
	if p == nil || len(hits) == 0 {
		return hits
	}
	sealedWL := map[string]bool{}
	if sw, ok := p.(SealedWorkloadsInNamespace); ok {
		if m, err := sw.SealedWorkloads(ctx, "all"); err == nil {
			for k, v := range m {
				if v {
					sealedWL[k] = true
					// also allow ns/name lookups
					if i := strings.LastIndex(k, "/"); i > 0 {
						sealedWL[k[:i]] = true // may collide; preferred keys are ns/kind/name from providers
					}
				}
			}
		}
	}
	sealedPod := map[string]bool{}
	if pods, err := p.ListPods(ctx, "all"); err == nil {
		for _, pod := range pods {
			if pod.Sealed || strings.EqualFold(pod.Status, "Sealed") {
				sealedPod[pod.Namespace+"/"+pod.Name] = true
				if pod.WorkloadID != "" {
					sealedWL[pod.Namespace+"/"+pod.WorkloadID] = true
				}
			}
		}
	}
	var out []FleetSearchHit
	for _, h := range hits {
		switch h.Kind {
		case FleetSearchWorkload:
			key := h.Namespace + "/" + h.Name
			keyKind := h.Namespace + "/" + strings.ToLower(h.WorkloadKind) + "/" + h.Name
			if sealedWL[key] || sealedWL[keyKind] || strings.EqualFold(h.Status, "Sealed") {
				h.Detail = "sealed · " + h.Detail
				out = append(out, h)
			}
		case FleetSearchPod:
			if sealedPod[h.Namespace+"/"+h.Name] || strings.EqualFold(h.Status, "Sealed") {
				h.Detail = "sealed · " + h.Detail
				out = append(out, h)
			}
		}
	}
	return out
}

// matchRank reports whether needle (lower-case) matches name or any of the
// other fields, and how well: 0 exact name, 1 name prefix, 2 name contains,
// 3 another field contains.
func matchRank(needle, name string, others ...string) (int, bool) {
	if needle == "" {
		return 3, true // sealed: alone lists all sealed hits
	}
	n := strings.ToLower(name)
	switch {
	case n == needle:
		return 0, true
	case strings.HasPrefix(n, needle):
		return 1, true
	case strings.Contains(n, needle):
		return 2, true
	}
	for _, o := range others {
		if strings.Contains(strings.ToLower(o), needle) {
			return 3, true
		}
	}
	return 0, false
}

var fleetKindOrder = map[string]int{FleetSearchWorkload: 0, FleetSearchPod: 1, FleetSearchService: 2, FleetSearchNamespace: 3}

func sortFleetHits(h []FleetSearchHit) {
	sort.SliceStable(h, func(i, j int) bool {
		a, b := h[i], h[j]
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		if a.cluster != b.cluster {
			return a.cluster < b.cluster
		}
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		if a.Kind != b.Kind {
			return fleetKindOrder[a.Kind] < fleetKindOrder[b.Kind]
		}
		return a.WorkloadKind < b.WorkloadKind
	})
}

// urlUserinfo matches the credentials part of a URL ("scheme://user:pass@").
var urlUserinfo = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^/\s@]+@`)

// fleetSearchError flattens an error for the UI and strips URL credentials.
func fleetSearchError(err error) string {
	return probeError(errors.New(urlUserinfo.ReplaceAllString(err.Error(), "$1")))
}
