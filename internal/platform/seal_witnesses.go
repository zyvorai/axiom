package platform

import (
	"fmt"
	"strings"
)

// SealWitnesses lists recent presence/audit actors who touched a workload —
// "who poked it" before a Seal (terminals, windows, exec).
func SealWitnesses(ns, kind, name string) []string {
	needle := strings.ToLower(ns + "/" + name)
	kindNeedle := strings.ToLower(ns + "/" + kind + "/" + name)
	seen := map[string]bool{}
	var out []string
	for _, a := range ListAxiomAudit() {
		tgt := strings.ToLower(a.Target)
		if !strings.Contains(tgt, needle) && !strings.Contains(tgt, kindNeedle) {
			continue
		}
		act := strings.ToLower(a.Action)
		if !(strings.Contains(act, "pod.terminal") || strings.Contains(act, "pod.window") ||
			strings.Contains(act, "pod.exec") || strings.Contains(act, "still") || strings.Contains(act, "seal")) {
			continue
		}
		who := strings.TrimSpace(a.User)
		if who == "" {
			who = "unknown"
		}
		key := who + "|" + a.Action
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, fmt.Sprintf("%s(%s)", who, a.Action))
		if len(out) >= 8 {
			break
		}
	}
	return out
}
