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

// kubeProxyIPVSFinding flags kube-proxy running in the deprecated ipvs mode,
// read from the kube-proxy ConfigMap's config.conf (kubeadm-style clusters).
// Distributions that do not keep that ConfigMap (k3s, managed control planes)
// produce no finding, which is "unknown", not "fine".
func kubeProxyIPVSFinding(cm ConfigMapDetail) []UpgradeFinding {
	for _, line := range strings.Split(cm.Data["config.conf"], "\n") {
		l := strings.TrimSpace(line)
		if !strings.HasPrefix(l, "mode:") {
			continue
		}
		mode := strings.Trim(strings.TrimSpace(strings.TrimPrefix(l, "mode:")), `"'`)
		if strings.EqualFold(mode, "ipvs") {
			return []UpgradeFinding{{
				Severity: "warning", Code: "kube-proxy-ipvs",
				Title:  "kube-proxy uses the deprecated ipvs mode",
				Detail: "kube-system/kube-proxy config.conf sets mode: ipvs",
				Hint:   "Plan a switch to nftables (or iptables) mode before ipvs is removed",
			}}
		}
	}
	return nil
}

func legacyUpgradeFindings(ctx context.Context, p Provider) []UpgradeFinding {
	var out []UpgradeFinding
	if ings, err := p.ListIngresses(ctx, ""); err == nil {
		out = append(out, ingressNginxFindings(ings)...)
	}
	if cm, err := p.GetConfigMap(ctx, "kube-system", "kube-proxy"); err == nil {
		out = append(out, kubeProxyIPVSFinding(cm)...)
	}
	return out
}
