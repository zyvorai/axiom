package platform

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// ingressNginxFindings flags Ingresses served by the retiring ingress-nginx
// controller (Kubernetes retired it in the 1.35 cycle) so operators can plan a
// move to Gateway API or another controller. Read-only; it never edits Ingresses.
func ingressNginxFindings(ings []IngressSummary) []UpgradeFinding {
	var hits []string
	for _, i := range ings {
		c := strings.ToLower(strings.TrimSpace(i.Class))
		if c == "nginx" || strings.Contains(c, "ingress-nginx") {
			hits = append(hits, i.Namespace+"/"+i.Name)
		}
	}
	if len(hits) == 0 {
		return nil
	}
	sort.Strings(hits)
	shown := hits
	if len(shown) > 5 {
		shown = shown[:5]
	}
	detail := fmt.Sprintf("%d Ingress(es) use the nginx class: %s", len(hits), strings.Join(shown, ", "))
	if len(hits) > len(shown) {
		detail += fmt.Sprintf(" and %d more", len(hits)-len(shown))
	}
	return []UpgradeFinding{{
		Severity: "warning", Code: "ingress-nginx-retiring",
		Title:  "ingress-nginx is being retired",
		Detail: detail,
		Hint:   "Plan a migration to Gateway API (see Networking → Gateways) or a maintained ingress controller",
	}}
}

func legacyUpgradeFindings(ctx context.Context, p Provider) []UpgradeFinding {
	ings, err := p.ListIngresses(ctx, "")
	if err != nil {
		return nil
	}
	return ingressNginxFindings(ings)
}
