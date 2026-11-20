package platform

import (
	"context"
	"strings"
)

// IntentDriftRow compares NL place annotation / Platform Intent CR / live replicas.
type IntentDriftRow struct {
	Namespace      string `json:"namespace"`
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	PlaceIntent    string `json:"placeIntent,omitempty"`    // axiom.zyvor.dev/intent annotation
	PlatformIntent string `json:"platformIntent,omitempty"` // CR name when present
	Desired        int32  `json:"desired"`
	Ready          int32  `json:"ready"`
	Drift          string `json:"drift"` // none | annotation-only | replicas | missing-cr | missing-annotation
	Note           string `json:"note,omitempty"`
}

// IntentDriftReport is GET /api/v1/platform-intents/drift.
type IntentDriftReport struct {
	Rows []IntentDriftRow `json:"rows"`
}

// ListIntentDrift builds a drift view from workloads + platform intents.
func ListIntentDrift(ctx context.Context, p Provider, ns string) (IntentDriftReport, error) {
	if ns == "" {
		ns = "all"
	}
	wls, err := p.ListWorkloads(ctx, ns)
	if err != nil {
		return IntentDriftReport{}, err
	}
	intents, _ := p.ListPlatformIntents(ctx, ns)
	byName := map[string]string{}
	for _, it := range intents {
		byName[it.Namespace+"/"+it.Name] = it.Name
	}
	// Place intent lives on workload annotations; ListPods surfaces it as Pod.Intent.
	intentByWL := map[string]string{}
	if pods, err := p.ListPods(ctx, ns); err == nil {
		for _, pod := range pods {
			if pod.Intent == "" {
				continue
			}
			key := pod.Namespace + "/"
			if pod.WorkloadID != "" {
				key += pod.WorkloadID
			} else {
				// Best-effort: strip replica suffix.
				name := pod.Name
				if i := strings.LastIndex(name, "-"); i > 0 {
					name = name[:i]
				}
				key += name
			}
			if _, ok := intentByWL[key]; !ok {
				intentByWL[key] = pod.Intent
			}
		}
	}
	var rows []IntentDriftRow
	for _, w := range wls {
		ann := ""
		if w.Labels != nil {
			ann = w.Labels[IntentAnnotation]
		}
		if ann == "" {
			ann = intentByWL[w.Namespace+"/"+w.Name]
		}
		cr := byName[w.Namespace+"/"+w.Name]
		row := IntentDriftRow{
			Namespace: w.Namespace, Name: w.Name, Kind: w.Kind,
			PlaceIntent: ann, PlatformIntent: cr,
			Desired: w.Desired, Ready: w.Ready, Drift: "none",
		}
		switch {
		case ann != "" && cr == "":
			row.Drift = "annotation-only"
			row.Note = "Place intent annotation present; no Platform Intent CR"
		case ann == "" && cr != "":
			row.Drift = "missing-annotation"
			row.Note = "Platform Intent CR exists without place annotation"
		case w.Desired != w.Ready && (ann != "" || cr != ""):
			row.Drift = "replicas"
			row.Note = "Intent-tracked workload is not at desired ready count"
		}
		if row.Drift != "none" || ann != "" || cr != "" {
			rows = append(rows, row)
		}
	}
	// CRs with no matching workload.
	seen := map[string]bool{}
	for _, r := range rows {
		seen[r.Namespace+"/"+r.Name] = true
	}
	for _, it := range intents {
		key := it.Namespace + "/" + it.Name
		if seen[key] {
			continue
		}
		rows = append(rows, IntentDriftRow{
			Namespace: it.Namespace, Name: it.Name, Kind: it.WorkloadKind,
			PlatformIntent: it.Name, Desired: it.Replicas, Drift: "missing-annotation",
			Note: "Platform Intent CR with no matching workload annotation",
		})
	}
	return IntentDriftReport{Rows: rows}, nil
}
