package platform

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// analyzeWeirdFindings adds finalizer ghosts, mutable image tags, and HPA thrash
// signals from provider list APIs (works for demo + any Provider).
func analyzeWeirdFindings(ctx context.Context, p Provider, ns string) []HealthFinding {
	var out []HealthFinding
	out = append(out, analyzeImageFloat(ctx, p, ns)...)
	out = append(out, analyzeHPAThrash(ctx, p, ns)...)
	return out
}

func analyzeImageFloat(ctx context.Context, p Provider, ns string) []HealthFinding {
	wls, err := p.ListWorkloads(ctx, ns)
	if err != nil {
		return nil
	}
	var out []HealthFinding
	for _, w := range wls {
		img := strings.TrimSpace(w.Image)
		if img == "" {
			continue
		}
		mutable := false
		switch {
		case strings.HasSuffix(img, ":latest"), strings.HasSuffix(img, ":main"), strings.HasSuffix(img, ":master"):
			mutable = true
		case !strings.Contains(img, "@sha256:") && !strings.Contains(img, ":"):
			mutable = true // implicit latest
		}
		if !mutable {
			continue
		}
		out = append(out, newFinding(AnalyzerImageFloat, SeverityInfo, w.Kind, w.Namespace, w.Name,
			"Mutable image tag",
			fmt.Sprintf("%s/%s runs %q — the tag can float to a different digest without a rollout notice.", w.Namespace, w.Name, img),
			"Pin the image by digest (@sha256:…) or an immutable semver tag so what runs is what you meant."))
	}
	return out
}

func analyzeHPAThrash(ctx context.Context, p Provider, ns string) []HealthFinding {
	hpas, err := p.ListHPAs(ctx, ns)
	if err != nil {
		return nil
	}
	evs, _ := p.ListEvents(ctx, ns)
	var out []HealthFinding
	for _, h := range hpas {
		ups, downs := 0, 0
		cutoff := time.Now().Add(-30 * time.Minute)
		for _, e := range evs {
			if e.Namespace != h.Namespace {
				continue
			}
			if !strings.Contains(e.Object, h.Name) && !strings.Contains(e.Object, "HorizontalPodAutoscaler") {
				continue
			}
			if e.Time.Before(cutoff) {
				continue
			}
			msg := strings.ToLower(e.Message + " " + e.Reason)
			if strings.Contains(msg, "scaled up") || strings.Contains(msg, "scaleup") {
				ups++
			}
			if strings.Contains(msg, "scaled down") || strings.Contains(msg, "scaledown") {
				downs++
			}
		}
		if ups >= 2 && downs >= 2 {
			out = append(out, newFinding(AnalyzerHPAThrash, SeverityWarning, "HorizontalPodAutoscaler", h.Namespace, h.Name,
				"HPA thrashing",
				fmt.Sprintf("HPA %s/%s scaled up %d and down %d times in ~30m — oscillation wastes churn and can flap readiness.", h.Namespace, h.Name, ups, downs),
				"Widen the scale-down stabilization window, raise thresholds, or fix the metric that is flapping."))
		}
	}
	return out
}
