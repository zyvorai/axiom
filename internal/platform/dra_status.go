package platform

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// enrichDRAObject fills Status / Detail / Driver from a DRA CR.
func enrichDRAObject(kind string, it unstructured.Unstructured, base DRAObject) DRAObject {
	switch kind {
	case "ResourceClaim":
		return enrichDRAClaim(it, base)
	case "ResourceSlice":
		node, _, _ := unstructured.NestedString(it.Object, "spec", "nodeName")
		driver, _, _ := unstructured.NestedString(it.Object, "spec", "driver")
		if driver != "" {
			base.Driver = driver
		}
		base.Status = "Published"
		if node != "" {
			base.Detail = "node=" + node
		}
		return base
	case "DeviceClass":
		selectors, _, _ := unstructured.NestedSlice(it.Object, "spec", "selectors")
		base.Status = "Available"
		if len(selectors) > 0 {
			base.Detail = fmt.Sprintf("%d selector(s)", len(selectors))
		}
		if base.Driver == "" {
			base.Driver, _, _ = unstructured.NestedString(it.Object, "spec", "suitableNodes") // rarely set
		}
		return base
	default:
		return base
	}
}

func enrichDRAClaim(it unstructured.Unstructured, base DRAObject) DRAObject {
	phase, _, _ := unstructured.NestedString(it.Object, "status", "phase")
	if phase == "" {
		// Older drafts used allocation presence instead of phase.
		if _, ok, _ := unstructured.NestedMap(it.Object, "status", "allocation"); ok {
			phase = "Allocated"
		} else {
			phase = "Pending"
		}
	}
	base.Status = phase

	var parts []string
	if driver, _, _ := unstructured.NestedString(it.Object, "status", "allocation", "devices", "config", "0", "opaque", "driver"); driver != "" {
		base.Driver = driver
	}
	// status.allocation.devices.results[].device / driver
	results, _, _ := unstructured.NestedSlice(it.Object, "status", "allocation", "devices", "results")
	var devices []string
	for _, r := range results {
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		if d, _ := m["driver"].(string); d != "" && base.Driver == "" {
			base.Driver = d
		}
		if id, _ := m["device"].(string); id != "" {
			devices = append(devices, id)
		}
	}
	if len(devices) > 0 {
		parts = append(parts, "devices="+strings.Join(devices, ","))
	}
	// Reserved / bound pod: status.reservedFor[].resource / name
	reserved, _, _ := unstructured.NestedSlice(it.Object, "status", "reservedFor")
	for _, r := range reserved {
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		res, _ := m["resource"].(string)
		if name == "" {
			continue
		}
		if res == "" || res == "pods" {
			parts = append(parts, "pod="+name)
		} else {
			parts = append(parts, res+"/"+name)
		}
		break
	}
	if len(parts) == 0 && phase != "" {
		parts = append(parts, "phase="+phase)
	}
	base.Detail = strings.Join(parts, " · ")
	return base
}
