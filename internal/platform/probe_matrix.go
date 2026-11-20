package platform

import (
	"context"
	"strings"
	"sync"
)

// ProbeMatrixRequest runs data-plane reachability checks from pods (or Axiom).
type ProbeMatrixRequest struct {
	URLs []string          `json:"urls"`
	Pods []ProbeMatrixPod  `json:"pods"`
}

type ProbeMatrixPod struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Container string `json:"container,omitempty"`
}

type ProbeMatrixCell struct {
	From      string `json:"from"` // "axiom" or "ns/pod"
	URL       string `json:"url"`
	OK        bool   `json:"ok"`
	LatencyMS int64  `json:"latencyMs"`
	Error     string `json:"error,omitempty"`
	Hint      string `json:"hint,omitempty"`
}

type ProbeMatrixResult struct {
	Cells []ProbeMatrixCell `json:"cells"`
}

// RunProbeMatrix probes each URL from each pod (capped) using TestPodProbe.
func RunProbeMatrix(ctx context.Context, provider Provider, base Integrations, req ProbeMatrixRequest) ProbeMatrixResult {
	urls := uniqueNonEmpty(req.URLs)
	if len(urls) == 0 && strings.TrimSpace(base.ProbeURL) != "" {
		urls = []string{base.ProbeURL}
	}
	if len(urls) > 8 {
		urls = urls[:8]
	}
	pods := req.Pods
	if len(pods) > 12 {
		pods = pods[:12]
	}
	if len(pods) == 0 {
		// Axiom-only row.
		pods = []ProbeMatrixPod{{}}
	}

	type job struct {
		pod ProbeMatrixPod
		url string
	}
	var jobs []job
	for _, p := range pods {
		for _, u := range urls {
			jobs = append(jobs, job{pod: p, url: u})
		}
	}

	cells := make([]ProbeMatrixCell, len(jobs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, j job) {
			defer wg.Done()
			defer func() { <-sem }()
			in := base
			in.ProbeURL = j.url
			in.ProbePodNamespace = strings.TrimSpace(j.pod.Namespace)
			in.ProbePodName = strings.TrimSpace(j.pod.Name)
			in.ProbePodContainer = strings.TrimSpace(j.pod.Container)
			res := TestPodProbe(ctx, provider, in)
			from := "axiom"
			if in.ProbePodNamespace != "" && in.ProbePodName != "" {
				from = in.ProbePodNamespace + "/" + in.ProbePodName
			}
			cells[i] = ProbeMatrixCell{
				From: from, URL: j.url, OK: res.OK, LatencyMS: res.LatencyMS,
				Error: res.Error, Hint: res.Hint,
			}
		}(i, j)
	}
	wg.Wait()
	return ProbeMatrixResult{Cells: cells}
}

func uniqueNonEmpty(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
