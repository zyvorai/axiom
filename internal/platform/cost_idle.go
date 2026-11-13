package platform

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Idle / zombie resource finder (GET /api/v1/cost/idle). Everything here is
// a heuristic over lists the Provider already serves, read-only: Axiom never
// deletes anything from this report, and a finding is a prompt to look, not
// proof the resource is unused.

const (
	// IdleDefaultMinAgeDays is the age below which nothing is reported.
	IdleDefaultMinAgeDays = 7
	// IdleMaxMinAgeDays caps the minAgeDays query parameter.
	IdleMaxMinAgeDays = 3650
	idleMaxPerKind    = 100

	// costStorageRatePerGiDefault prices PVC capacity (USD per GiB-month).
	// The request-based cost model has no storage line, so this is a flat
	// estimate; override with AXIOM_COST_STORAGE_RATE.
	costStorageRatePerGiDefault = 0.10
)

const (
	IdleKindPVC        = "PersistentVolumeClaim"
	IdleKindNamespace  = "Namespace"
	IdleKindService    = "Service"
	IdleKindJob        = "Job"
	IdleKindDeployment = "Deployment"
	IdleKindConfigMap  = "ConfigMap"
	IdleKindSecret     = "Secret"
)

func costStorageRatePerGi() float64 {
	v := strings.TrimSpace(os.Getenv("AXIOM_COST_STORAGE_RATE"))
	if v == "" {
		return costStorageRatePerGiDefault
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		return costStorageRatePerGiDefault
	}
	return f
}

// IdleFinding is one suspected idle resource. Values of ConfigMaps and
// Secrets are never read, only their names.
type IdleFinding struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	Reason    string `json:"reason"`
	Age       string `json:"age"`
	AgeDays   int    `json:"ageDays"`
	// Confidence is high, medium or low: how likely the resource really is idle.
	Confidence string `json:"confidence"`
	// MonthlyUSD is set only where a rate exists (PVC capacity).
	MonthlyUSD *float64 `json:"monthlyUsd,omitempty"`
	// Suggestion is advice text; Axiom performs no action from this report.
	Suggestion string `json:"suggestion"`
}

type IdleSummary struct {
	Total             int            `json:"total"`
	ByKind            map[string]int `json:"byKind"`
	MonthlySavingsUSD float64        `json:"monthlySavingsUsd"`
}

type IdleReport struct {
	MinAgeDays int           `json:"minAgeDays"`
	Namespace  string        `json:"namespace,omitempty"`
	Summary    IdleSummary   `json:"summary"`
	Findings   []IdleFinding `json:"findings"`
	Truncated  bool          `json:"truncated,omitempty"`
	Note       string        `json:"note"`
}

// IdleInput is the cluster snapshot DetectIdle works on.
type IdleInput struct {
	Volumes    []Volume
	Pods       []Pod
	Namespaces []Namespace
	Workloads  []Workload
	Services   []Service
	Jobs       []JobSummary
	ConfigMaps []ConfigMapSummary
	Secrets    []SecretSummary
	// PodRefsKnown is false when pod mount/reference data is unavailable;
	// ConfigMap/Secret/PVC-mount findings are then skipped.
	PodRefsKnown bool
}

// idleAgeDays reads the age strings the providers emit ("now", "5m", "3h",
// "12d"). ok is false for anything else.
func idleAgeDays(s string) (days int, ok bool) {
	s = strings.TrimSpace(s)
	if s == "now" {
		return 0, true
	}
	if len(s) < 2 {
		return 0, false
	}
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil || n < 0 {
		return 0, false
	}
	switch s[len(s)-1] {
	case 'm', 'h':
		return 0, true
	case 'd':
		return n, true
	}
	return 0, false
}

func idleMoney(v float64) float64 { return math.Round(v*100) / 100 }

// DetectIdle is pure: it only reads in. namespace "" or "all" means every
// namespace; minAgeDays <= 0 falls back to the default.
func DetectIdle(in IdleInput, namespace string, minAgeDays int) IdleReport {
	if minAgeDays <= 0 {
		minAgeDays = IdleDefaultMinAgeDays
	}
	if namespace == "all" {
		namespace = ""
	}
	inScope := func(ns string) bool { return namespace == "" || ns == namespace }
	old := func(age string) (int, bool) {
		d, ok := idleAgeDays(age)
		return d, ok && d >= minAgeDays
	}

	mounted := map[string]bool{}
	cmRef := map[string]bool{}
	secRef := map[string]bool{}
	podNS := map[string]bool{}
	for _, p := range in.Pods {
		podNS[p.Namespace] = true
		for _, v := range p.PVCs {
			mounted[p.Namespace+"/"+v] = true
		}
		for _, v := range p.ConfigMaps {
			cmRef[p.Namespace+"/"+v] = true
		}
		for _, v := range p.Secrets {
			secRef[p.Namespace+"/"+v] = true
		}
	}
	wlNS := map[string]bool{}
	for _, w := range in.Workloads {
		wlNS[w.Namespace] = true
	}

	rate := costStorageRatePerGi()
	var out []IdleFinding
	add := func(f IdleFinding) { out = append(out, f) }

	// PVCs: unbound, or bound but mounted by no pod.
	for _, v := range in.Volumes {
		if !inScope(v.Namespace) {
			continue
		}
		days, ok := old(v.Age)
		if !ok {
			continue
		}
		gi := float64(parseMemoryBytes(v.Capacity)) / (1 << 30)
		var cost *float64
		if gi > 0 {
			c := idleMoney(gi * rate)
			cost = &c
		}
		switch {
		case v.Status != "" && v.Status != "Bound":
			add(IdleFinding{Kind: IdleKindPVC, Namespace: v.Namespace, Name: v.Name, Age: v.Age, AgeDays: days,
				Reason:     fmt.Sprintf("Claim is %s (not bound to a volume) and at least %d days old.", v.Status, minAgeDays),
				Confidence: "high", MonthlyUSD: cost,
				Suggestion: "Check why it never bound (StorageClass, capacity, access mode); delete the claim if nothing needs it."})
		case in.PodRefsKnown && !mounted[v.Namespace+"/"+v.Name]:
			add(IdleFinding{Kind: IdleKindPVC, Namespace: v.Namespace, Name: v.Name, Age: v.Age, AgeDays: days,
				Reason:     "Bound claim that no Pod currently mounts.",
				Confidence: "medium", MonthlyUSD: cost,
				Suggestion: "Confirm no scaled-down StatefulSet or scheduled job needs it, take a backup, then delete the claim."})
		}
	}

	// Empty namespaces.
	for _, n := range in.Namespaces {
		if !inScope(n.Name) || protectedNamespace(n.Name) || strings.HasPrefix(n.Name, "kube-") || n.Status == "Terminating" {
			continue
		}
		days, ok := old(n.Age)
		if !ok || podNS[n.Name] || wlNS[n.Name] || n.Pods > 0 {
			continue
		}
		add(IdleFinding{Kind: IdleKindNamespace, Name: n.Name, Age: n.Age, AgeDays: days,
			Reason:     "No Pods or workloads in the namespace.",
			Confidence: "medium",
			Suggestion: "Check for remaining Services, PVCs, Secrets or CRDs you still want, then delete the namespace if it is abandoned."})
	}

	// LoadBalancer Services with no ready endpoints.
	for _, s := range in.Services {
		if !inScope(s.Namespace) || s.Type != "LoadBalancer" || len(s.Endpoints) > 0 {
			continue
		}
		days, ok := old(s.Age)
		if !ok {
			continue
		}
		add(IdleFinding{Kind: IdleKindService, Namespace: s.Namespace, Name: s.Name, Age: s.Age, AgeDays: days,
			Reason:     "LoadBalancer Service with no ready endpoints; the cloud load balancer may still be billed.",
			Confidence: "medium",
			Suggestion: "Fix the selector or workload, or delete the Service to release the load balancer."})
	}

	// Finished Jobs.
	for _, j := range in.Jobs {
		if !inScope(j.Namespace) || (j.Status != "Complete" && j.Status != "Failed") {
			continue
		}
		days, ok := old(j.Age)
		if !ok {
			continue
		}
		conf := "high"
		if j.Status == "Failed" {
			conf = "medium"
		}
		add(IdleFinding{Kind: IdleKindJob, Namespace: j.Namespace, Name: j.Name, Age: j.Age, AgeDays: days,
			Reason:     fmt.Sprintf("Job is %s and at least %d days old.", strings.ToLower(j.Status), minAgeDays),
			Confidence: conf,
			Suggestion: "Review its logs if it failed, then delete it or set ttlSecondsAfterFinished so it cleans itself up."})
	}

	// Scaled-to-zero Deployments.
	for _, w := range in.Workloads {
		if !inScope(w.Namespace) || w.Kind != "Deployment" || w.Desired != 0 {
			continue
		}
		days, ok := old(w.Age)
		if !ok {
			continue
		}
		add(IdleFinding{Kind: IdleKindDeployment, Namespace: w.Namespace, Name: w.Name, Age: w.Age, AgeDays: days,
			Reason:     "Deployment is scaled to zero replicas.",
			Confidence: "low",
			Suggestion: "It may be parked on purpose (or sealed); delete it if it will not be scaled back up."})
	}

	// Unreferenced ConfigMaps / Secrets (advisory; names only).
	if in.PodRefsKnown {
		for _, c := range in.ConfigMaps {
			if !inScope(c.Namespace) || protectedNamespace(c.Namespace) || c.Name == "kube-root-ca.crt" || cmRef[c.Namespace+"/"+c.Name] {
				continue
			}
			if days, ok := old(c.Age); ok {
				add(IdleFinding{Kind: IdleKindConfigMap, Namespace: c.Namespace, Name: c.Name, Age: c.Age, AgeDays: days,
					Reason:     "No Pod references this ConfigMap.",
					Confidence: "low",
					Suggestion: "Advisory only: operators, CRDs and scaled-down workloads may still use it. Verify before removing."})
			}
		}
		for _, s := range in.Secrets {
			if !inScope(s.Namespace) || protectedNamespace(s.Namespace) || secRef[s.Namespace+"/"+s.Name] ||
				strings.HasPrefix(s.Type, "helm.sh/") || s.Type == "kubernetes.io/service-account-token" {
				continue
			}
			if days, ok := old(s.Age); ok {
				add(IdleFinding{Kind: IdleKindSecret, Namespace: s.Namespace, Name: s.Name, Age: s.Age, AgeDays: days,
					Reason:     "No Pod references this Secret.",
					Confidence: "low",
					Suggestion: "Advisory only: Ingress TLS, ServiceAccounts, operators and scaled-down workloads may still use it. Verify before removing."})
			}
		}
	}

	money := func(f IdleFinding) float64 {
		if f.MonthlyUSD == nil {
			return 0
		}
		return *f.MonthlyUSD
	}
	sort.SliceStable(out, func(i, j int) bool {
		if mi, mj := money(out[i]), money(out[j]); mi != mj {
			return mi > mj
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})

	rep := IdleReport{MinAgeDays: minAgeDays, Namespace: namespace, Findings: []IdleFinding{},
		Note: "Heuristics from current cluster state, not guarantees: verify each finding before acting. Axiom never deletes anything from this report."}
	rep.Summary.ByKind = map[string]int{}
	perKind := map[string]int{}
	for _, f := range out {
		rep.Summary.ByKind[f.Kind]++
		rep.Summary.Total++
		rep.Summary.MonthlySavingsUSD += money(f)
		perKind[f.Kind]++
		if perKind[f.Kind] > idleMaxPerKind {
			rep.Truncated = true
			continue
		}
		rep.Findings = append(rep.Findings, f)
	}
	rep.Summary.MonthlySavingsUSD = idleMoney(rep.Summary.MonthlySavingsUSD)
	return rep
}

// IdleResources gathers the lists from provider and runs DetectIdle.
func IdleResources(ctx context.Context, provider Provider, namespace string, minAgeDays int) (IdleReport, error) {
	var in IdleInput
	var err error
	if in.Volumes, err = provider.ListVolumes(ctx, ""); err != nil {
		return IdleReport{}, err
	}
	if in.Pods, err = provider.ListPods(ctx, ""); err != nil {
		return IdleReport{}, err
	}
	if in.Namespaces, err = provider.ListNamespaces(ctx); err != nil {
		return IdleReport{}, err
	}
	if in.Workloads, err = provider.ListWorkloads(ctx, ""); err != nil {
		return IdleReport{}, err
	}
	if in.Services, err = provider.ListServices(ctx, ""); err != nil {
		return IdleReport{}, err
	}
	if in.Jobs, err = provider.ListJobs(ctx, ""); err != nil {
		return IdleReport{}, err
	}
	// Advisory lists are best-effort: a forbidden list just drops that kind.
	in.ConfigMaps, _ = provider.ListConfigMaps(ctx, "")
	in.Secrets, _ = provider.ListSecrets(ctx, "")
	in.PodRefsKnown = true
	if isDemoProvider(ctx, provider) {
		applyDemoIdle(&in)
	}
	return DetectIdle(in, namespace, minAgeDays), nil
}

func isDemoProvider(ctx context.Context, p Provider) bool {
	if sp, ok := p.(*SwitchableProvider); ok {
		p = sp.current(ctx)
	}
	for i := 0; i < 8 && p != nil; i++ {
		switch v := p.(type) {
		case *clusterLabeledProvider:
			p = v.Provider
		case *simulatedClusterProvider:
			p = v.Provider
		case *DemoProvider:
			return true
		default:
			return false
		}
	}
	return false
}

// applyDemoIdle seeds a few stale resources into the demo snapshot (demo
// pods carry no mount data) so the Idle resources section has content.
func applyDemoIdle(in *IdleInput) {
	in.Pods = append(in.Pods, Pod{Namespace: "data", Name: "postgres-0", PVCs: []string{"postgres-data"}})
	in.Namespaces = append(in.Namespaces, Namespace{Name: "sandbox-old", Status: "Active", Age: "41d"})
	in.Services = append(in.Services, Service{Namespace: "labs", Name: "preview-lb", Type: "LoadBalancer", Age: "19d", Endpoints: []string{}})
	in.Jobs = append(in.Jobs, JobSummary{Namespace: "commerce", Name: "migrate-schema-7731", Status: "Failed", Completions: "0/1", Age: "23d"})
	in.Workloads = append(in.Workloads, Workload{Namespace: "labs", Name: "legacy-worker", Kind: "Deployment", Desired: 0, Age: "60d", Status: "Stopped"})
	in.ConfigMaps = append(in.ConfigMaps, ConfigMapSummary{Namespace: "commerce", Name: "old-feature-flags", KeyCount: 3, Age: "75d"})
}
