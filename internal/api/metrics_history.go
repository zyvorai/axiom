package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/zyvorai/axiom/internal/platform"
)

func (s *Server) metricsHistory(w http.ResponseWriter, r *http.Request) {
	ns := strings.TrimSpace(r.URL.Query().Get("namespace"))
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	kind := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("kind")))
	if kind == "" {
		kind = "pod"
	}
	window := strings.TrimSpace(r.URL.Query().Get("window"))
	if window == "" {
		window = "1h"
	}
	dur := time.Hour
	switch window {
	case "6h":
		dur = 6 * time.Hour
	case "24h":
		dur = 24 * time.Hour
	case "1h":
		dur = time.Hour
	default:
		fail(w, http.StatusBadRequest, errSSO("window must be 1h, 6h or 24h"))
		return
	}
	client := platform.PrometheusFromProvider(s.provider)
	if client == nil {
		writeJSON(w, http.StatusOK, platform.MetricHistory{
			Available: false,
			Window:    window,
			Note:      "Set AXIOM_PROMETHEUS_URL to enable historical CPU/memory charts",
		})
		return
	}
	end := time.Now()
	start := end.Add(-dur)
	step := dur / 60
	if step < 30*time.Second {
		step = 30 * time.Second
	}

	var cpuQ, memQ string
	switch kind {
	case "node":
		sel := `instance!=""`
		if name != "" {
			sel = fmt.Sprintf(`node="%s"`, name)
		}
		cpuQ = `sum by (node) (rate(node_cpu_seconds_total{mode!="idle",` + sel + `}[5m]))`
		memQ = `sum by (node) (node_memory_MemTotal_bytes{` + sel + `} - node_memory_MemAvailable_bytes{` + sel + `})`
		// Fallback to kubelet cAdvisor node-ish series if node_exporter is absent:
		if name != "" {
			cpuQ = `sum(rate(container_cpu_usage_seconds_total{container!="",container!="POD",node="` + name + `"}[5m]))`
			memQ = `sum(container_memory_working_set_bytes{container!="",container!="POD",node="` + name + `"})`
		}
	case "workload", "deployment", "statefulset", "daemonset":
		sel := `namespace!=""`
		if ns != "" && ns != "all" {
			sel = `namespace="` + ns + `"`
		}
		if name != "" {
			sel += `,pod=~"` + name + `.*"`
		}
		cpuQ = `sum by (pod) (rate(container_cpu_usage_seconds_total{container!="",container!="POD",` + sel + `}[5m]))`
		memQ = `sum by (pod) (container_memory_working_set_bytes{container!="",container!="POD",` + sel + `})`
	default: // pod
		sel := `namespace!=""`
		if ns != "" && ns != "all" {
			sel = `namespace="` + ns + `"`
		}
		if name != "" {
			sel += `,pod=~"` + name + `.*"`
		}
		cpuQ = `sum by (pod) (rate(container_cpu_usage_seconds_total{container!="",container!="POD",` + sel + `}[5m]))`
		memQ = `sum by (pod) (container_memory_working_set_bytes{container!="",container!="POD",` + sel + `})`
	}

	cpu, err := client.QueryRange(r.Context(), cpuQ, start, end, step)
	if err != nil {
		writeJSON(w, http.StatusOK, platform.MetricHistory{Available: true, Window: window, Note: err.Error()})
		return
	}
	mem, err := client.QueryRange(r.Context(), memQ, start, end, step)
	if err != nil {
		writeJSON(w, http.StatusOK, platform.MetricHistory{Available: true, Window: window, Note: err.Error()})
		return
	}
	series := make([]platform.MetricHistorySeries, 0, len(cpu)+len(mem)+4)
	for _, s := range cpu {
		s.Name = "cpu:" + s.Name
		series = append(series, s)
	}
	for _, s := range mem {
		s.Name = "mem:" + s.Name
		series = append(series, s)
	}
	// Dual-family DCGM GPU util when kind=node (classic % or renamed ratio).
	if kind == "node" && name != "" {
		gpuQueries := []struct {
			prefix string
			q      string
		}{
			{"gpu:", `avg(DCGM_FI_DEV_GPU_UTIL{Hostname="` + name + `"})`},
			{"gpu:", `avg(DCGM_FI_DEV_GPU_UTIL{node="` + name + `"})`},
			{"gpu:", `avg(DCGM_FI_DEV_GPU_UTIL_RATIO{Hostname="` + name + `"})*100`},
			{"gpu:", `avg(DCGM_FI_DEV_GPU_UTIL_RATIO{node="` + name + `"})*100`},
		}
		for _, gq := range gpuQueries {
			gs, gerr := client.QueryRange(r.Context(), gq.q, start, end, step)
			if gerr != nil || len(gs) == 0 {
				continue
			}
			for _, s := range gs {
				s.Name = gq.prefix + s.Name
				series = append(series, s)
			}
			break
		}
	}
	writeJSON(w, http.StatusOK, platform.MetricHistory{Available: true, Window: window, Series: series})
}
