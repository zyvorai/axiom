package platform

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Rates EstimateCost prices requested resources at (USD per month). The
// rightsizing recommender prices its deltas with the same rates so a
// recommendation's savings line up with the Cost tab's totals.
const (
	costCPURatePerCore     = 20.0
	costMemoryRatePerGi    = 5.0
	costGPURatePerGpuDefault = 100.0
)

// costGPURatePerGpu returns the estimate rate for nvidia.com/gpu (+ MIG keys).
// Override with AXIOM_COST_GPU_RATE (USD per GPU per month).
func costGPURatePerGpu() float64 {
	v := strings.TrimSpace(os.Getenv("AXIOM_COST_GPU_RATE"))
	if v == "" {
		return costGPURatePerGpuDefault
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		return costGPURatePerGpuDefault
	}
	return f
}

// costMonthlyUSD prices cpu cores and memory GiB with the EstimateCost rates.
func costMonthlyUSD(cpuCores, memGi float64) float64 {
	return cpuCores*costCPURatePerCore + memGi*costMemoryRatePerGi
}

func isGPURequestKey(k string) bool {
	k = strings.ToLower(k)
	return k == "nvidia.com/gpu" || strings.HasPrefix(k, "nvidia.com/mig-")
}

// ---------------------------------------------------------------------------
// Report types (GET /api/v1/cost/rightsizing)

type RightsizingReport struct {
	// Source is the usage data the recommendations rest on: prometheus,
	// vpa, metrics-snapshot or none.
	Source     string             `json:"source"`
	Window     string             `json:"window"`
	Confidence string             `json:"confidence"`
	Note       string             `json:"note"`
	Summary    RightsizingSummary `json:"summary"`
	Items      []RightsizingItem  `json:"items"`
}

type RightsizingSummary struct {
	// Workloads counts distinct workloads with at least one assessed container.
	Workloads int `json:"workloads"`
	// OverProvisioned / UnderProvisioned count container rows by verdict.
	OverProvisioned  int `json:"overProvisioned"`
	UnderProvisioned int `json:"underProvisioned"`
	// MonthlySavingsUSD sums the savings of the "over" rows (a positive number).
	MonthlySavingsUSD float64 `json:"monthlySavingsUsd"`
}

type RightsizingItem struct {
	Namespace string `json:"namespace"`
	Workload  string `json:"workload"`
	Kind      string `json:"kind"`
	Container string `json:"container"`
	// Replicas is how many running pods the workload has; monthlyDeltaUsd
	// covers all of them, like EstimateCost does.
	Replicas int               `json:"replicas"`
	CPU      RightsizingCPU    `json:"cpu"`
	Memory   RightsizingMemory `json:"memory"`
	// Verdict is over, under or ok.
	Verdict string `json:"verdict"`
	// MonthlyDeltaUSD is (recommended - request) priced per core / GiB for
	// every replica. Negative means savings.
	MonthlyDeltaUSD float64 `json:"monthlyDeltaUsd"`
	Reason          string  `json:"reason"`
}

type RightsizingCPU struct {
	RequestMilli     int64  `json:"requestMilli"`
	RecommendedMilli int64  `json:"recommendedMilli"`
	UsageMilli       int64  `json:"usageMilli"`
	Basis            string `json:"basis"`
}

type RightsizingMemory struct {
	RequestMiB     int64  `json:"requestMiB"`
	RecommendedMiB int64  `json:"recommendedMiB"`
	UsageMiB       int64  `json:"usageMiB"`
	Basis          string `json:"basis"`
}

const (
	rightsizingSourcePrometheus = "prometheus"
	rightsizingSourceSnapshot   = "metrics-snapshot"
	rightsizingSourceNone       = "none"

	rightsizingDefaultWindow = "7d"
	rightsizingMaxItems      = 500

	mib = int64(1024 * 1024)
)

// rightsizingWindows are the accepted ?window= values and their length in days.
var rightsizingWindows = map[string]int{"1d": 1, "3d": 3, "7d": 7, "14d": 14, "30d": 30}

// NormalizeRightsizingWindow returns the window to use ("" means 7d) or a
// 400 error for anything outside 1d, 3d, 7d, 14d, 30d.
func NormalizeRightsizingWindow(w string) (string, error) {
	w = strings.TrimSpace(w)
	if w == "" {
		return rightsizingDefaultWindow, nil
	}
	if _, ok := rightsizingWindows[w]; !ok {
		return "", NewStatusError(http.StatusBadRequest, fmt.Errorf("window must be one of 1d, 3d, 7d, 14d, 30d"))
	}
	return w, nil
}

// ---------------------------------------------------------------------------
// Recommendation math

// recommendCPUMilli is usage x 1.2 headroom, rounded up to 10m, at least 10m.
func recommendCPUMilli(usageMilli int64) int64 {
	if usageMilli < 0 {
		usageMilli = 0
	}
	// ceil(usage*1.2 / 10) * 10, in integers so 100m -> 120m exactly.
	rec := (usageMilli*12 + 99) / 100 * 10
	if rec < 10 {
		rec = 10
	}
	return rec
}

// recommendMemoryMiB is usage x 1.15 headroom, rounded up to 8 MiB, at least 16 MiB.
func recommendMemoryMiB(usageBytes int64) int64 {
	if usageBytes < 0 {
		usageBytes = 0
	}
	// ceil(usage*1.15 / 8MiB) * 8, in integers.
	const step = 8 * mib
	scaled := usageBytes * 115 // bytes x 100
	rec := (scaled + step*100 - 1) / (step * 100) * 8
	if rec < 16 {
		rec = 16
	}
	return rec
}

func ceilMiB(b int64) int64 {
	if b <= 0 {
		return 0
	}
	return (b + mib - 1) / mib
}

// rightsizingContainer is one container of a workload with its per-replica requests.
type rightsizingContainer struct {
	Namespace, Kind, Workload, Container string
	Replicas                             int
	RequestCPUMilli                      int64 // 0 = no request set
	RequestMemBytes                      int64 // 0 = no request set
}

func (c rightsizingContainer) key() string {
	return c.Namespace + "/" + c.Kind + "/" + c.Workload + "/" + c.Container
}

// containerUsage is the observed usage of one workload container (the peak
// across its pods for the chosen statistic).
type containerUsage struct {
	CPUMilli int64
	MemBytes int64
	HasCPU   bool
	HasMem   bool
}

// rightsizingBasis names the statistic each resource's usage was taken from.
type rightsizingBasis struct{ CPU, Memory string }

// recommendContainer applies the rightsizing rules to one container. ok is
// false when there is nothing to say (no usage known for it).
func recommendContainer(c rightsizingContainer, u containerUsage, basis rightsizingBasis) (RightsizingItem, bool) {
	if !u.HasCPU && !u.HasMem {
		return RightsizingItem{}, false
	}
	replicas := c.Replicas
	if replicas < 1 {
		replicas = 1
	}
	usageMiB := ceilMiB(u.MemBytes)
	reqMiB := ceilMiB(c.RequestMemBytes)
	item := RightsizingItem{
		Namespace: c.Namespace, Workload: c.Workload, Kind: c.Kind, Container: c.Container, Replicas: replicas,
		CPU:    RightsizingCPU{RequestMilli: c.RequestCPUMilli, UsageMilli: u.CPUMilli, Basis: basis.CPU},
		Memory: RightsizingMemory{RequestMiB: reqMiB, UsageMiB: usageMiB, Basis: basis.Memory},
	}
	// Recommend only for a resource whose usage is known; otherwise keep the request.
	if u.HasCPU {
		item.CPU.RecommendedMilli = recommendCPUMilli(u.CPUMilli)
	} else {
		item.CPU.RecommendedMilli = c.RequestCPUMilli
	}
	if u.HasMem {
		item.Memory.RecommendedMiB = recommendMemoryMiB(u.MemBytes)
	} else {
		item.Memory.RecommendedMiB = reqMiB
	}
	cpuDelta := float64(item.CPU.RecommendedMilli-item.CPU.RequestMilli) / 1000
	memDelta := float64(item.Memory.RecommendedMiB-item.Memory.RequestMiB) / 1024
	item.MonthlyDeltaUSD = round2(costMonthlyUSD(cpuDelta, memDelta) * float64(replicas))

	noCPUReq := c.RequestCPUMilli <= 0
	noMemReq := c.RequestMemBytes <= 0
	switch {
	case noCPUReq && noMemReq:
		item.Verdict, item.Reason = "under", "no request set"
		return item, true
	case noCPUReq && u.HasCPU:
		item.Verdict, item.Reason = "under", "no CPU request set"
		return item, true
	case noMemReq && u.HasMem:
		item.Verdict, item.Reason = "under", "no memory request set"
		return item, true
	}

	// Under-provisioned first: usage close to the request risks throttling / OOM.
	var risks []string
	if u.HasCPU && c.RequestCPUMilli > 0 && float64(u.CPUMilli) > 0.9*float64(c.RequestCPUMilli) {
		risks = append(risks, fmt.Sprintf("CPU usage %dm is %.0f%% of the %dm request (throttling risk)",
			u.CPUMilli, pct(float64(u.CPUMilli), float64(c.RequestCPUMilli)), c.RequestCPUMilli))
	}
	if u.HasMem && c.RequestMemBytes > 0 && float64(u.MemBytes) > 0.9*float64(c.RequestMemBytes) {
		risks = append(risks, fmt.Sprintf("memory usage %dMi is %.0f%% of the %dMi request (OOM risk)",
			usageMiB, pct(float64(u.MemBytes), float64(c.RequestMemBytes)), reqMiB))
	}
	if len(risks) > 0 {
		item.Verdict, item.Reason = "under", strings.Join(risks, "; ")
		return item, true
	}

	var over []string
	if u.HasCPU && c.RequestCPUMilli > 0 {
		rec := item.CPU.RecommendedMilli
		if float64(c.RequestCPUMilli) > 1.5*float64(rec) && c.RequestCPUMilli-rec >= 50 {
			over = append(over, fmt.Sprintf("CPU request %dm is %.1fx the recommended %dm", c.RequestCPUMilli, float64(c.RequestCPUMilli)/float64(rec), rec))
		}
	}
	if u.HasMem && c.RequestMemBytes > 0 {
		rec := item.Memory.RecommendedMiB
		if float64(reqMiB) > 1.5*float64(rec) && reqMiB-rec >= 64 {
			over = append(over, fmt.Sprintf("memory request %dMi is %.1fx the recommended %dMi", reqMiB, float64(reqMiB)/float64(rec), rec))
		}
	}
	if len(over) > 0 {
		item.Verdict, item.Reason = "over", strings.Join(over, "; ")
		return item, true
	}
	item.Verdict, item.Reason = "ok", "requests are in line with observed usage"
	return item, true
}

func pct(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b * 100
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// finishRightsizingItems sorts (savings first, then namespace / workload /
// container), computes the summary over every row and caps the list.
func finishRightsizingItems(items []RightsizingItem) ([]RightsizingItem, RightsizingSummary) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.MonthlyDeltaUSD != b.MonthlyDeltaUSD {
			return a.MonthlyDeltaUSD < b.MonthlyDeltaUSD
		}
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		if a.Workload != b.Workload {
			return a.Workload < b.Workload
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Container < b.Container
	})
	var sum RightsizingSummary
	workloads := map[string]bool{}
	var savings float64
	for _, it := range items {
		workloads[it.Namespace+"/"+it.Kind+"/"+it.Workload] = true
		switch it.Verdict {
		case "over":
			sum.OverProvisioned++
			if it.MonthlyDeltaUSD < 0 {
				savings -= it.MonthlyDeltaUSD
			}
		case "under":
			sum.UnderProvisioned++
		}
	}
	sum.Workloads = len(workloads)
	sum.MonthlySavingsUSD = round2(savings)
	if len(items) > rightsizingMaxItems {
		items = items[:rightsizingMaxItems]
	}
	if items == nil {
		items = []RightsizingItem{}
	}
	return items, sum
}

// ---------------------------------------------------------------------------
// Usage sources

// podContainerUsage is one container sample as a usage source reports it.
type podContainerUsage struct {
	Namespace, Pod, Container string
	CPUMilli                  int64
	MemBytes                  int64
	HasCPU, HasMem            bool
}

// rightsizingUsageSource is where per-container usage comes from. The
// recommender tries sources in order (Prometheus history, then a
// metrics.k8s.io snapshot) and reports the first that yields data. A VPA
// source would be another implementation returning the VPA target per
// container with basis "vpa-target".
type rightsizingUsageSource interface {
	// name is the report's source value.
	name() string
	basis() rightsizingBasis
	confidence(window string) string
	note(window string) string
	usage(ctx context.Context, namespace, window string) ([]podContainerUsage, error)
}

// snapshotUsageSource is a single metrics.k8s.io sample of current usage.
type snapshotUsageSource struct {
	fetch func(ctx context.Context, namespace string) ([]podContainerUsage, error)
}

func (snapshotUsageSource) name() string { return rightsizingSourceSnapshot }
func (snapshotUsageSource) basis() rightsizingBasis {
	return rightsizingBasis{CPU: "current", Memory: "current"}
}
func (snapshotUsageSource) confidence(string) string { return "low" }
func (snapshotUsageSource) note(string) string {
	return "Based on a single metrics-server sample of current usage. One sample is not a usage profile: treat these as low-confidence hints. Set AXIOM_PROMETHEUS_URL for p95/p99 recommendations over a time window."
}
func (s snapshotUsageSource) usage(ctx context.Context, namespace, _ string) ([]podContainerUsage, error) {
	if s.fetch == nil {
		return nil, fmt.Errorf("metrics.k8s.io is not available")
	}
	return s.fetch(ctx, namespace)
}

// rightsizingInputs is what the provider gathered from the cluster.
type rightsizingInputs struct {
	Containers []rightsizingContainer
	// Resolve maps a pod (possibly gone, as Prometheus history includes old
	// pods) to its workload. ok=false drops the sample.
	Resolve func(namespace, pod string) (kind, name string, ok bool)
}

// runRightsizing picks the first usage source that yields samples, maps the
// samples to workload containers and runs the recommender. It never fails:
// source errors become a note and the next source is tried.
func runRightsizing(ctx context.Context, namespace, window string, sources []rightsizingUsageSource, in rightsizingInputs) RightsizingReport {
	rep := RightsizingReport{Window: window, Items: []RightsizingItem{}}
	src, usage, notes := selectRightsizingUsage(ctx, namespace, window, sources, in)
	if src != nil {
		items := make([]RightsizingItem, 0, len(in.Containers))
		for _, c := range in.Containers {
			if it, ok := recommendContainer(c, usage[c.key()], src.basis()); ok {
				items = append(items, it)
			}
		}
		rep.Source = src.name()
		rep.Confidence = src.confidence(window)
		rep.Items, rep.Summary = finishRightsizingItems(items)
		rep.Note = strings.Join(append(notes, src.note(window)), " ")
		return rep
	}
	rep.Source, rep.Confidence = rightsizingSourceNone, "low"
	_, rep.Summary = finishRightsizingItems(nil)
	notes = append(notes, "No usage data is available, so there are no recommendations. Point AXIOM_PROMETHEUS_URL at a Prometheus that scrapes the kubelet's cAdvisor metrics (container_cpu_usage_seconds_total, container_memory_working_set_bytes) for p95/p99 history, or install metrics-server for a current-usage snapshot.")
	rep.Note = strings.Join(notes, " ")
	return rep
}

// selectRightsizingUsage returns the first source that yields usage for a
// running workload container, with that usage, and notes on the sources
// skipped. src is nil when none did.
func selectRightsizingUsage(ctx context.Context, namespace, window string, sources []rightsizingUsageSource, in rightsizingInputs) (rightsizingUsageSource, map[string]containerUsage, []string) {
	var notes []string
	for _, src := range sources {
		samples, err := src.usage(ctx, namespace, window)
		if err != nil {
			notes = append(notes, fmt.Sprintf("%s unavailable: %s.", sourceLabel(src.name()), err.Error()))
			continue
		}
		usage := aggregateUsage(samples, in.Resolve)
		if len(usage) == 0 {
			notes = append(notes, fmt.Sprintf("%s returned no container usage for running workloads.", sourceLabel(src.name())))
			continue
		}
		return src, usage, notes
	}
	return nil, nil, notes
}

// recommendOneContainer runs the same pipeline as runRightsizing (same source
// choice, same window) for one container of in. ok is false when the
// container is not among in's running containers or no usage is known for it.
func recommendOneContainer(ctx context.Context, window string, sources []rightsizingUsageSource, in rightsizingInputs, key string) (RightsizingItem, rightsizingUsageSource, bool) {
	var target *rightsizingContainer
	for i := range in.Containers {
		if in.Containers[i].key() == key {
			target = &in.Containers[i]
			break
		}
	}
	if target == nil {
		return RightsizingItem{}, nil, false
	}
	src, usage, _ := selectRightsizingUsage(ctx, target.Namespace, window, sources, in)
	if src == nil {
		return RightsizingItem{}, nil, false
	}
	it, ok := recommendContainer(*target, usage[key], src.basis())
	return it, src, ok
}

func sourceLabel(name string) string {
	switch name {
	case rightsizingSourcePrometheus:
		return "Prometheus"
	case rightsizingSourceSnapshot:
		return "metrics.k8s.io"
	}
	return name
}

// aggregateUsage folds pod samples into workload containers, keeping the
// highest value across replicas (the replica that needs the most decides).
func aggregateUsage(samples []podContainerUsage, resolve func(ns, pod string) (string, string, bool)) map[string]containerUsage {
	out := map[string]containerUsage{}
	for _, s := range samples {
		if resolve == nil || s.Container == "" || s.Container == "POD" {
			continue
		}
		kind, name, ok := resolve(s.Namespace, s.Pod)
		if !ok {
			continue
		}
		k := s.Namespace + "/" + kind + "/" + name + "/" + s.Container
		cur := out[k]
		if s.HasCPU {
			cur.HasCPU = true
			if s.CPUMilli > cur.CPUMilli {
				cur.CPUMilli = s.CPUMilli
			}
		}
		if s.HasMem {
			cur.HasMem = true
			if s.MemBytes > cur.MemBytes {
				cur.MemBytes = s.MemBytes
			}
		}
		out[k] = cur
	}
	return out
}

var (
	podSuffixDeployment  = regexp.MustCompile(`^(.+)-[a-z0-9]{1,10}-[a-z0-9]{5}$`)
	podSuffixStatefulSet = regexp.MustCompile(`^(.+)-[0-9]+$`)
	podSuffixDaemonSet   = regexp.MustCompile(`^(.+)-[a-z0-9]{5}$`)
)

// workloadFromPodName applies the standard controller pod-name suffix rules
// (Deployment "<name>-<rs hash>-<5>", StatefulSet "<name>-<ordinal>",
// DaemonSet "<name>-<5>") and accepts a candidate only when known reports
// that workload exists (key "namespace/kind/name").
func workloadFromPodName(namespace, pod string, known map[string]bool) (kind, name string, ok bool) {
	try := func(re *regexp.Regexp, k string) bool {
		if m := re.FindStringSubmatch(pod); m != nil && known[namespace+"/"+k+"/"+m[1]] {
			kind, name = k, m[1]
			return true
		}
		return false
	}
	if try(podSuffixDeployment, "Deployment") || try(podSuffixStatefulSet, "StatefulSet") || try(podSuffixDaemonSet, "DaemonSet") {
		return kind, name, true
	}
	return "", "", false
}
