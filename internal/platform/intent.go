package platform

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type Floor string

const (
	FloorProd    Floor = "prod"
	FloorStaging Floor = "staging"
	FloorPlant   Floor = "plant"
)

type PlaceScores struct {
	Cost    int `json:"cost"`
	Fit     int `json:"fit"`
	Latency int `json:"latency"`
	Policy  int `json:"policy"`
}

type PlaceDraft struct {
	Intent   string      `json:"intent"`
	Name     string      `json:"name"`
	Replicas int32       `json:"replicas"`
	Floor    Floor       `json:"floor"`
	Namespace string     `json:"namespace"`
	Memory   string      `json:"memory"`
	NoRoot   bool        `json:"noRoot"`
	Image    string      `json:"image"`
	Scores   PlaceScores `json:"scores"`
}

func NamespaceForFloor(f Floor) string {
	switch f {
	case FloorPlant:
		return "kube-system"
	case FloorStaging:
		return "staging"
	default:
		return "default"
	}
}

func FloorForNamespace(ns string) Floor {
	low := strings.ToLower(ns)
	switch {
	case ns == "kube-system" || ns == "network" || ns == "axiom-system":
		return FloorPlant
	case strings.Contains(low, "staging") || ns == "migrate":
		return FloorStaging
	default:
		return FloorProd
	}
}

func ParseIntent(raw string) PlaceDraft {
	text := strings.TrimSpace(raw)
	if text == "" {
		text = "Run demo-api, 1 copy, on prod, no root, 256Mi"
	}
	name := "workload"
	if m := regexp.MustCompile(`(?i)run\s+([a-z0-9][a-z0-9-]*)`).FindStringSubmatch(text); len(m) > 1 {
		name = strings.ToLower(m[1])
	} else {
		var b strings.Builder
		for _, r := range strings.ToLower(text) {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				b.WriteRune(r)
			} else {
				b.WriteByte('-')
			}
		}
		name = strings.Trim(b.String(), "-")
		if len(name) > 24 {
			name = name[:24]
		}
		if name == "" {
			name = "workload"
		}
	}
	replicas := int32(1)
	if m := regexp.MustCompile(`(?i)(\d+)\s*(copies|copy|replicas|pods)`).FindStringSubmatch(text); len(m) > 1 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			replicas = int32(n)
		}
	}
	if replicas < 1 {
		replicas = 1
	}
	if replicas > 8 {
		replicas = 8
	}
	floor := FloorProd
	low := strings.ToLower(text)
	if strings.Contains(low, "staging") {
		floor = FloorStaging
	} else if strings.Contains(low, "plant") || strings.Contains(low, "kube-system") {
		floor = FloorPlant
	}
	memory := "256Mi"
	if m := regexp.MustCompile(`(?i)(\d+\s*mi|\d+\s*gi)`).FindStringSubmatch(text); len(m) > 1 {
		memory = strings.ReplaceAll(m[1], " ", "")
	}
	noRoot := !strings.Contains(low, "privileged")
	image := "nginx:1.27"
	if m := regexp.MustCompile(`(?i)(?:image|img)\s+([a-z0-9./:_-]+)`).FindStringSubmatch(text); len(m) > 1 {
		image = m[1]
	} else if m := regexp.MustCompile(`(?i)\b([a-z0-9]+/[a-z0-9._-]+:[a-z0-9._-]+)\b`).FindStringSubmatch(text); len(m) > 1 {
		image = m[1]
	}
	fit := 88
	if floor == FloorPlant {
		fit = 54
	}
	lat := 93
	if floor == FloorProd {
		lat = 81
	}
	pol := 96
	if !noRoot {
		pol = 38
	}
	cost := 100 - int(replicas)*11
	if cost < 18 {
		cost = 18
	}
	return PlaceDraft{
		Intent: text, Name: name, Replicas: replicas, Floor: floor,
		Namespace: NamespaceForFloor(floor), Memory: memory, NoRoot: noRoot,
		Image: image,
		Scores: PlaceScores{Cost: cost, Fit: fit, Latency: lat, Policy: pol},
	}
}

func (d PlaceDraft) LaunchRequest() LaunchRequest {
	port := int32(80)
	return LaunchRequest{
		Namespace: d.Namespace, Name: d.Name, Image: d.Image, Replicas: d.Replicas,
		Port: port, CPU: "250m", Memory: d.Memory, Environment: map[string]string{}, Expose: true,
	}
}

func (d PlaceDraft) ReceiptYAML() string {
	return strings.Join([]string{
		"apiVersion: axiom.zyvor.dev/v1",
		"kind: Application",
		"metadata:",
		"  name: " + d.Name,
		"  namespace: " + d.Namespace,
		"  annotations:",
		fmt.Sprintf("    axiom.zyvor.dev/intent: %q", d.Intent),
		"spec:",
		"  runtime: kubernetes",
		fmt.Sprintf("  replicas: %d", d.Replicas),
		"  image: " + d.Image,
		"  resources:",
		"    memory: " + d.Memory,
		"  securityContext:",
		fmt.Sprintf("    runAsNonRoot: %t", d.NoRoot),
	}, "\n")
}
