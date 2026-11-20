package platform

import (
	"os"
	"strings"
)

// DefaultRegions mirrors the Emergent multi-region selector while keeping the
// live cluster identity available as a first-class choice.
func DefaultRegions(cluster string) []Region {
	active := strings.TrimSpace(os.Getenv("AXIOM_REGION"))
	raw := strings.TrimSpace(os.Getenv("AXIOM_REGIONS"))
	ids := []string{"eu-west-1", "eu-central-1", "us-east-1"}
	if raw != "" {
		ids = nil
		for _, part := range strings.Split(raw, ",") {
			id := strings.TrimSpace(part)
			if id != "" {
				ids = append(ids, id)
			}
		}
	}
	if cluster == "" {
		cluster = "zyvor"
	}
	hasCluster := false
	for _, id := range ids {
		if id == cluster {
			hasCluster = true
			break
		}
	}
	if !hasCluster {
		ids = append([]string{cluster}, ids...)
	}
	if active == "" {
		active = "eu-west-1"
		for _, id := range ids {
			if id == cluster {
				continue
			}
			active = id
			break
		}
	}
	out := make([]Region, 0, len(ids))
	seen := map[string]struct{}{}
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		label := id
		if id == cluster {
			label = cluster + " · cluster"
		}
		out = append(out, Region{ID: id, Label: label, Active: id == active})
	}
	if len(out) == 0 {
		return []Region{{ID: cluster, Label: cluster, Active: true}}
	}
	hasActive := false
	for _, r := range out {
		if r.Active {
			hasActive = true
			break
		}
	}
	if !hasActive {
		out[0].Active = true
	}
	return out
}

func ActiveRegion(regions []Region) string {
	for _, r := range regions {
		if r.Active {
			return r.ID
		}
	}
	if len(regions) > 0 {
		return regions[0].ID
	}
	return "eu-west-1"
}
