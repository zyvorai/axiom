package platform

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/yaml"
)

// VerticalPodAutoscaler and KEDA ScaledObject creation: typed requests,
// validation, rendering and the KEDA trigger allow-list.
//
// This file is pure (no cluster access) and compiles in both build modes: the
// preview endpoints call PreviewVPA / PreviewScaledObject directly, and both
// providers call buildVPA / buildScaledObject before creating anything. Every
// object is assembled field by field from the typed request; raw YAML is never
// accepted. Trigger settings are a closed, per-type allow-list of short
// strings, and credentials are never accepted in them: they go through
// authenticationRef, the NAME of an existing TriggerAuthentication.
//
// Error messages name fields and setting keys but never echo setting values,
// which could be credentials pasted into the wrong field.

const (
	autoscalerManagedByLabel = "app.kubernetes.io/managed-by"
	autoscalerManagedByValue = "zyvor-axiom"

	vpaMaxCPUCores            = 256
	kedaMaxTriggers           = 10
	kedaMaxSettingLen         = 512
	kedaDefaultPolling  int32 = 30
	kedaDefaultCooldown int32 = 300
	kedaMaxPolling      int32 = 3600
	kedaMaxCooldown     int32 = 86400
)

var (
	vpaMaxMemory      = resource.MustParse("4Ti")
	vpaTargetKinds    = map[string]bool{"Deployment": true, "StatefulSet": true, "DaemonSet": true, "ReplicaSet": true}
	vpaUpdateModes    = []string{"Off", "Initial", "Recreate", "Auto"}
	kedaTargetKinds   = map[string]bool{"Deployment": true, "StatefulSet": true}
	kedaSafeTokenRE   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
	kedaCronFieldRE   = regexp.MustCompile(`^[0-9A-Za-z*/,?-]{1,64}$`)
	kedaTimezoneRE    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_+/-]{0,63}$`)
	kedaHostPortRE    = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?(:[0-9]{1,5})?$`)
	kedaAWSRegionRE   = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-[0-9]{1,2}$`)
	kedaCredentialKey = regexp.MustCompile(`(?i)password|passwd|token|secret|key|credential|auth`)
)

// AutoscalerObjectRef identifies one rendered or created object.
type AutoscalerObjectRef struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
}

// AutoscalerPreview is the response of the VPA and ScaledObject preview routes.
type AutoscalerPreview struct {
	Objects  []AutoscalerObjectRef `json:"objects"`
	YAML     string                `json:"yaml"`
	Warnings []string              `json:"warnings"`
}

// AutoscalerCreateResult is the 201 response of POST /vpas and /scaledobjects.
type AutoscalerCreateResult struct {
	OK       bool                  `json:"ok"`
	Created  []AutoscalerObjectRef `json:"created"`
	Warnings []string              `json:"warnings"`
}

// VPAContainerPolicyRequest bounds the recommendations for one container
// ("*" for all). Each quantity is optional.
type VPAContainerPolicyRequest struct {
	Name      string `json:"name"`
	MinCPU    string `json:"minCpu,omitempty"`
	MaxCPU    string `json:"maxCpu,omitempty"`
	MinMemory string `json:"minMemory,omitempty"`
	MaxMemory string `json:"maxMemory,omitempty"`
}

// VPACreateRequest is the body of POST /api/v1/vpas and /api/v1/vpas/preview.
type VPACreateRequest struct {
	Namespace           string                      `json:"namespace"`
	Name                string                      `json:"name"`
	TargetKind          string                      `json:"targetKind"`
	TargetName          string                      `json:"targetName"`
	UpdateMode          string                      `json:"updateMode,omitempty"`
	ControlledResources []string                    `json:"controlledResources,omitempty"`
	Containers          []VPAContainerPolicyRequest `json:"containers,omitempty"`
}

// ScaledObjectTriggerRequest is one KEDA trigger. Settings become the
// trigger's metadata, except `type` of cpu/memory which becomes metricType.
type ScaledObjectTriggerRequest struct {
	Type              string            `json:"type"`
	Name              string            `json:"name,omitempty"`
	Settings          map[string]string `json:"settings"`
	AuthenticationRef string            `json:"authenticationRef,omitempty"`
}

// ScaledObjectCreateRequest is the body of POST /api/v1/scaledobjects and
// /api/v1/scaledobjects/preview. MinReplicas is a pointer because 0 (scale to
// zero) is meaningful; when absent it defaults to 0 if an event trigger is
// present and 1 otherwise. PollingInterval and CooldownPeriod default to
// KEDA's 30 and 300 seconds when 0.
type ScaledObjectCreateRequest struct {
	Namespace       string                       `json:"namespace"`
	Name            string                       `json:"name"`
	TargetKind      string                       `json:"targetKind"`
	TargetName      string                       `json:"targetName"`
	MinReplicas     *int32                       `json:"minReplicas,omitempty"`
	MaxReplicas     int32                        `json:"maxReplicas"`
	PollingInterval int32                        `json:"pollingInterval,omitempty"`
	CooldownPeriod  int32                        `json:"cooldownPeriod,omitempty"`
	Triggers        []ScaledObjectTriggerRequest `json:"triggers"`
}

// TriggerAuthenticationSummary is a KEDA TriggerAuthentication reduced to its
// name: its spec (secret references, pod identity) is never read out.
type TriggerAuthenticationSummary struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Kind      string `json:"kind"` // always "TriggerAuthentication"
	Age       string `json:"age"`
}

// TriggerAuthenticationList is the /api/v1/triggerauthentications response.
type TriggerAuthenticationList struct {
	Installed bool                           `json:"installed"`
	Items     []TriggerAuthenticationSummary `json:"items"`
}

// autoscalerObject is one rendered object. Group/Version/Resource identify
// the CRD; Version is the newest known one, which the Kubernetes provider
// replaces with the version the API server actually serves.
type autoscalerObject struct {
	Group, Version, Resource, Kind string
	Namespace, Name                string
	Obj                            map[string]any
}

func (o autoscalerObject) ref(version string) AutoscalerObjectRef {
	return AutoscalerObjectRef{APIVersion: o.Group + "/" + version, Kind: o.Kind, Namespace: o.Namespace, Name: o.Name}
}

func (o autoscalerObject) withVersion(version string) map[string]any {
	doc := make(map[string]any, len(o.Obj)+1)
	for k, v := range o.Obj {
		doc[k] = v
	}
	doc["apiVersion"] = o.Group + "/" + version
	return doc
}

type autoscalerPlan struct {
	Object   autoscalerObject
	Warnings []string
	// ScaledObject only: TriggerAuthentication names to check at create time.
	AuthRefs []string
	// For the HPA conflict checks.
	TargetKind, TargetName string
	Resources              []string // VPA: controlled resources
}

func (p autoscalerPlan) preview() (AutoscalerPreview, error) {
	b, err := yaml.Marshal(p.Object.withVersion(p.Object.Version))
	if err != nil {
		return AutoscalerPreview{}, err
	}
	return AutoscalerPreview{
		Objects:  []AutoscalerObjectRef{p.Object.ref(p.Object.Version)},
		YAML:     string(b),
		Warnings: nonNilStrings(p.Warnings),
	}, nil
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func autoscalerMeta(ns, name string) map[string]any {
	return map[string]any{
		"name":      name,
		"namespace": ns,
		"labels":    map[string]any{autoscalerManagedByLabel: autoscalerManagedByValue},
	}
}

// checkAutoscalerTarget validates the fields every autoscaler request shares.
func checkAutoscalerTarget(ns, name, kind, target string, kinds map[string]bool, kindList string) error {
	if ns == "" || name == "" || kind == "" || target == "" {
		return hpaBadRequest("namespace, name, targetKind, and targetName are required")
	}
	if !hpaDNSLabelRE.MatchString(ns) {
		return hpaBadRequest("invalid namespace %q: must be a DNS-1123 label", ns)
	}
	if len(name) > 253 || !hpaDNSSubdomainRE.MatchString(name) {
		return hpaBadRequest("invalid name %q: must be a DNS-1123 subdomain", name)
	}
	if len(target) > 253 || !hpaDNSSubdomainRE.MatchString(target) {
		return hpaBadRequest("invalid targetName %q: must be a DNS-1123 subdomain", target)
	}
	if !kinds[kind] {
		return hpaBadRequest("invalid targetKind %q: must be %s", safeEcho(kind), kindList)
	}
	return nil
}

// safeEcho returns v when it is a plain short token, otherwise a placeholder,
// so error messages never reflect arbitrary client input.
func safeEcho(v string) string {
	if kedaSafeTokenRE.MatchString(v) {
		return v
	}
	return "(invalid)"
}

// --- VerticalPodAutoscaler ---

func parseVPAQuantity(field, v string, cpu bool) (*resource.Quantity, error) {
	if v == "" {
		return nil, nil
	}
	if len(v) > 32 {
		return nil, hpaBadRequest("%s is not a valid quantity", field)
	}
	q, err := resource.ParseQuantity(v)
	if err != nil {
		return nil, hpaBadRequest("%s is not a valid quantity (e.g. 250m or 2 for cpu, 512Mi or 2Gi for memory)", field)
	}
	if q.Sign() <= 0 {
		return nil, hpaBadRequest("%s must be positive", field)
	}
	if cpu && q.Cmp(*resource.NewQuantity(vpaMaxCPUCores, resource.DecimalSI)) > 0 {
		return nil, hpaBadRequest("%s must be at most %d cores", field, vpaMaxCPUCores)
	}
	if !cpu && q.Cmp(vpaMaxMemory) > 0 {
		return nil, hpaBadRequest("%s must be at most 4Ti", field)
	}
	return &q, nil
}

// buildVPA validates req and renders the VerticalPodAutoscaler.
func buildVPA(req VPACreateRequest) (autoscalerPlan, error) {
	if err := checkAutoscalerTarget(req.Namespace, req.Name, req.TargetKind, req.TargetName, vpaTargetKinds, "Deployment, StatefulSet, DaemonSet or ReplicaSet"); err != nil {
		return autoscalerPlan{}, err
	}
	mode := req.UpdateMode
	if mode == "" {
		mode = "Off"
	}
	validMode := false
	for _, m := range vpaUpdateModes {
		validMode = validMode || m == mode
	}
	if !validMode {
		return autoscalerPlan{}, hpaBadRequest("invalid updateMode %q: must be Off, Initial, Recreate or Auto", safeEcho(mode))
	}

	controlled := req.ControlledResources
	if len(controlled) == 0 {
		controlled = []string{"cpu", "memory"}
	}
	seenRes := map[string]bool{}
	for _, r := range controlled {
		if r != "cpu" && r != "memory" {
			return autoscalerPlan{}, hpaBadRequest("invalid controlledResources entry %q: must be cpu or memory", safeEcho(r))
		}
		if seenRes[r] {
			return autoscalerPlan{}, hpaBadRequest("controlledResources lists %s twice", r)
		}
		seenRes[r] = true
	}
	controlledAny := make([]any, 0, len(controlled))
	for _, r := range controlled {
		controlledAny = append(controlledAny, r)
	}

	if len(req.Containers) > 50 {
		return autoscalerPlan{}, hpaBadRequest("at most 50 container policies are allowed")
	}
	policies := []any{}
	seenC := map[string]bool{}
	for i, c := range req.Containers {
		field := fmt.Sprintf("containers[%d]", i)
		if c.Name == "" {
			return autoscalerPlan{}, hpaBadRequest("%s.name is required (a container name, or * for all containers)", field)
		}
		if c.Name != "*" && !hpaDNSLabelRE.MatchString(c.Name) {
			return autoscalerPlan{}, hpaBadRequest("%s.name %q must be a DNS-1123 label or *", field, safeEcho(c.Name))
		}
		if seenC[c.Name] {
			return autoscalerPlan{}, hpaBadRequest("%s.name %q is listed twice", field, c.Name)
		}
		seenC[c.Name] = true
		minCPU, err := parseVPAQuantity(field+".minCpu", c.MinCPU, true)
		if err != nil {
			return autoscalerPlan{}, err
		}
		maxCPU, err := parseVPAQuantity(field+".maxCpu", c.MaxCPU, true)
		if err != nil {
			return autoscalerPlan{}, err
		}
		minMem, err := parseVPAQuantity(field+".minMemory", c.MinMemory, false)
		if err != nil {
			return autoscalerPlan{}, err
		}
		maxMem, err := parseVPAQuantity(field+".maxMemory", c.MaxMemory, false)
		if err != nil {
			return autoscalerPlan{}, err
		}
		if minCPU != nil && maxCPU != nil && minCPU.Cmp(*maxCPU) > 0 {
			return autoscalerPlan{}, hpaBadRequest("%s.minCpu must not exceed maxCpu", field)
		}
		if minMem != nil && maxMem != nil && minMem.Cmp(*maxMem) > 0 {
			return autoscalerPlan{}, hpaBadRequest("%s.minMemory must not exceed maxMemory", field)
		}
		if !seenRes["cpu"] && (minCPU != nil || maxCPU != nil) {
			return autoscalerPlan{}, hpaBadRequest("%s sets cpu bounds but cpu is not in controlledResources", field)
		}
		if !seenRes["memory"] && (minMem != nil || maxMem != nil) {
			return autoscalerPlan{}, hpaBadRequest("%s sets memory bounds but memory is not in controlledResources", field)
		}
		p := map[string]any{"containerName": c.Name, "controlledResources": controlledAny}
		bounds := func(cpu, mem *resource.Quantity) map[string]any {
			m := map[string]any{}
			if cpu != nil {
				m["cpu"] = cpu.String()
			}
			if mem != nil {
				m["memory"] = mem.String()
			}
			return m
		}
		if m := bounds(minCPU, minMem); len(m) > 0 {
			p["minAllowed"] = m
		}
		if m := bounds(maxCPU, maxMem); len(m) > 0 {
			p["maxAllowed"] = m
		}
		policies = append(policies, p)
	}
	// Restricting the controlled resources without per-container bounds still
	// needs a policy to carry the restriction.
	if len(policies) == 0 && len(controlled) < 2 {
		policies = append(policies, map[string]any{"containerName": "*", "controlledResources": controlledAny})
	}

	spec := map[string]any{
		"targetRef":    map[string]any{"apiVersion": "apps/v1", "kind": req.TargetKind, "name": req.TargetName},
		"updatePolicy": map[string]any{"updateMode": mode},
	}
	if len(policies) > 0 {
		spec["resourcePolicy"] = map[string]any{"containerPolicies": policies}
	}
	plan := autoscalerPlan{
		Object: autoscalerObject{
			Group: "autoscaling.k8s.io", Version: "v1", Resource: "verticalpodautoscalers", Kind: "VerticalPodAutoscaler",
			Namespace: req.Namespace, Name: req.Name,
			Obj: map[string]any{
				"kind":     "VerticalPodAutoscaler",
				"metadata": autoscalerMeta(req.Namespace, req.Name),
				"spec":     spec,
			},
		},
		Warnings:   []string{},
		TargetKind: req.TargetKind, TargetName: req.TargetName,
		Resources: controlled,
	}
	if mode != "Off" {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("updateMode %s: pods may be restarted to apply new requests (Off only records recommendations)", mode))
	}
	return plan, nil
}

// PreviewVPA validates req and renders what CreateVPA would create (newest
// API version). It never talks to a cluster, so the HPA conflict warning is
// only reported by create.
func PreviewVPA(req VPACreateRequest) (AutoscalerPreview, error) {
	plan, err := buildVPA(req)
	if err != nil {
		return AutoscalerPreview{}, err
	}
	return plan.preview()
}

// hpaScalesOn reports which of resources an HPA's metrics scale on.
func hpaScalesOn(h HPASummary, resources []string) []string {
	want := map[string]bool{}
	for _, r := range resources {
		want[r] = true
	}
	hit := map[string]bool{}
	if h.TargetCPUPercent > 0 && want["cpu"] {
		hit["cpu"] = true
	}
	for _, m := range h.Metrics {
		if (m.Type == "Resource" || m.Type == "ContainerResource") && want[m.Name] {
			hit[m.Name] = true
		}
	}
	out := make([]string, 0, len(hit))
	for r := range hit {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// vpaHPAConflictWarnings warns about HPAs that scale the same workload on a
// resource the VPA controls.
func vpaHPAConflictWarnings(plan autoscalerPlan, hpas []HPASummary) []string {
	var out []string
	for _, h := range hpas {
		if h.Namespace != plan.Object.Namespace || h.TargetKind != plan.TargetKind || h.TargetName != plan.TargetName {
			continue
		}
		if on := hpaScalesOn(h, plan.Resources); len(on) > 0 {
			out = append(out, fmt.Sprintf("VPA and HPA on the same resource conflict: HPA %s already scales %s/%s on %s",
				h.Name, plan.TargetKind, plan.TargetName, strings.Join(on, " and ")))
		}
	}
	return out
}

// kedaHPAConflictWarnings warns about plain (non-KEDA) HPAs on the workload.
func kedaHPAConflictWarnings(plan autoscalerPlan, hpas []HPASummary) []string {
	var out []string
	for _, h := range hpas {
		if h.Namespace != plan.Object.Namespace || h.TargetKind != plan.TargetKind || h.TargetName != plan.TargetName {
			continue
		}
		if strings.HasPrefix(h.Name, "keda-hpa-") {
			continue // owned by a ScaledObject
		}
		out = append(out, fmt.Sprintf("KEDA creates its own HPA; two autoscalers on one workload fight each other: HPA %s already targets %s/%s",
			h.Name, plan.TargetKind, plan.TargetName))
	}
	return out
}

// --- KEDA ScaledObject ---

// kedaTriggerType is the allow-list entry for one trigger type.
type kedaTriggerType struct {
	required []string
	optional []string
	// check validates one present setting value (already length/control
	// checked). Keys missing here only get the generic checks.
	check map[string]func(string) error
	// auth: "forbidden", "optional" or "required" for authenticationRef.
	auth string
	// metricType: settings["type"] becomes the trigger's metricType instead of
	// a metadata entry (cpu, memory).
	metricType bool
	// note explains a rule in the unknown-key error (e.g. where host goes).
	note string
}

func (t kedaTriggerType) allowed() []string {
	out := append(append([]string{}, t.required...), t.optional...)
	sort.Strings(out)
	return out
}

func (t kedaTriggerType) has(k string) bool {
	for _, x := range t.required {
		if x == k {
			return true
		}
	}
	for _, x := range t.optional {
		if x == k {
			return true
		}
	}
	return false
}

func checkEnum(vals ...string) func(string) error {
	return func(v string) error {
		for _, x := range vals {
			if v == x {
				return nil
			}
		}
		return fmt.Errorf("must be one of %s", strings.Join(vals, ", "))
	}
}

func checkPositiveInt(max int64) func(string) error {
	return func(v string) error {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 1 || n > max {
			return fmt.Errorf("must be a whole number between 1 and %d", max)
		}
		return nil
	}
}

func checkNonNegInt(v string) error {
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return fmt.Errorf("must be a whole number of at least 0")
	}
	return nil
}

func checkNumber(allowZero bool) func(string) error {
	return func(v string) error {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 0 || (!allowZero && f == 0) || math.IsNaN(f) || math.IsInf(f, 0) {
			if allowZero {
				return fmt.Errorf("must be a number of at least 0")
			}
			return fmt.Errorf("must be a positive number")
		}
		return nil
	}
}

func checkHTTPURL(v string) error {
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("must be an http:// or https:// URL")
	}
	if u.User != nil {
		return fmt.Errorf("must not contain credentials; use authenticationRef")
	}
	return nil
}

func checkHTTPSURL(v string) error {
	if err := checkHTTPURL(v); err != nil {
		return err
	}
	if !strings.HasPrefix(v, "https://") {
		return fmt.Errorf("must be an https:// URL")
	}
	return nil
}

func checkHostPort(v string) error {
	if strings.Contains(v, "@") {
		return fmt.Errorf("must not contain credentials; use authenticationRef")
	}
	if !kedaHostPortRE.MatchString(v) {
		return fmt.Errorf("must be host or host:port")
	}
	return nil
}

func checkHostPortList(v string) error {
	parts := strings.Split(v, ",")
	if len(parts) > 20 {
		return fmt.Errorf("must list at most 20 brokers")
	}
	for _, p := range parts {
		if err := checkHostPort(strings.TrimSpace(p)); err != nil {
			return fmt.Errorf("must be a comma-separated list of host:port: %v", err)
		}
	}
	return nil
}

func checkCron(v string) error {
	f := strings.Fields(v)
	if len(f) != 5 {
		return fmt.Errorf("must be a 5-field cron expression (minute hour day month weekday)")
	}
	for _, x := range f {
		if !kedaCronFieldRE.MatchString(x) {
			return fmt.Errorf("must be a 5-field cron expression (minute hour day month weekday)")
		}
	}
	return nil
}

func checkTimezone(v string) error {
	if !kedaTimezoneRE.MatchString(v) || strings.Contains(v, "..") {
		return fmt.Errorf("must be an IANA time zone such as Europe/Berlin or UTC")
	}
	return nil
}

func checkToken(v string) error {
	if !kedaSafeTokenRE.MatchString(v) && !(len(v) <= 253 && hpaDNSSubdomainRE.MatchString(v)) {
		return fmt.Errorf("must be a name of letters, digits, '.', '_' and '-'")
	}
	return nil
}

func checkResourceTarget(v string) error {
	n, err := strconv.ParseInt(v, 10, 64)
	if err == nil && n >= 1 && n <= 1000 {
		return nil
	}
	if q, qerr := resource.ParseQuantity(v); qerr == nil && q.Sign() > 0 {
		return nil
	}
	return fmt.Errorf("must be a percentage (1-1000) for Utilization or a positive quantity for AverageValue")
}

// kedaTriggerTypes is the closed set of trigger types the form can create.
var kedaTriggerTypes = map[string]kedaTriggerType{
	"cpu": {
		required: []string{"type", "value"}, auth: "forbidden", metricType: true,
		check: map[string]func(string) error{"type": checkEnum("Utilization", "AverageValue"), "value": checkResourceTarget},
	},
	"memory": {
		required: []string{"type", "value"}, auth: "forbidden", metricType: true,
		check: map[string]func(string) error{"type": checkEnum("Utilization", "AverageValue"), "value": checkResourceTarget},
	},
	"cron": {
		required: []string{"timezone", "start", "end", "desiredReplicas"}, auth: "forbidden",
		check: map[string]func(string) error{
			"timezone": checkTimezone, "start": checkCron, "end": checkCron, "desiredReplicas": checkPositiveInt(1000),
		},
	},
	"prometheus": {
		required: []string{"serverAddress", "query", "threshold"}, optional: []string{"activationThreshold", "namespace"}, auth: "optional",
		check: map[string]func(string) error{
			"serverAddress": checkHTTPURL, "threshold": checkNumber(false), "activationThreshold": checkNumber(true),
			"namespace": func(v string) error {
				if !hpaDNSLabelRE.MatchString(v) {
					return fmt.Errorf("must be a DNS-1123 label")
				}
				return nil
			},
		},
	},
	"kafka": {
		required: []string{"bootstrapServers", "consumerGroup", "topic", "lagThreshold"}, optional: []string{"activationLagThreshold"}, auth: "optional",
		check: map[string]func(string) error{
			"bootstrapServers": checkHostPortList, "consumerGroup": checkToken, "topic": checkToken,
			"lagThreshold": checkPositiveInt(1 << 40), "activationLagThreshold": checkNonNegInt,
		},
	},
	"redis": {
		required: []string{"address", "listName", "listLength"}, optional: []string{"activationListLength"}, auth: "optional",
		check: map[string]func(string) error{
			"address": checkHostPort, "listName": checkToken,
			"listLength": checkPositiveInt(1 << 40), "activationListLength": checkNonNegInt,
		},
	},
	"rabbitmq": {
		required: []string{"queueName", "mode", "value"}, optional: []string{"protocol"}, auth: "required",
		note: "the RabbitMQ host (it carries the password) is supplied only through authenticationRef",
		check: map[string]func(string) error{
			"queueName": checkToken, "mode": checkEnum("QueueLength", "MessageRate"), "value": checkNumber(false),
			"protocol": checkEnum("auto", "amqp", "http"),
		},
	},
	"aws-sqs-queue": {
		required: []string{"queueURL", "queueLength", "awsRegion"}, auth: "optional",
		check: map[string]func(string) error{
			"queueURL": checkHTTPSURL, "queueLength": checkPositiveInt(1 << 40),
			"awsRegion": func(v string) error {
				if !kedaAWSRegionRE.MatchString(v) {
					return fmt.Errorf("must be an AWS region such as eu-central-1")
				}
				return nil
			},
		},
	},
}

// kedaCredentialKeyAllow are allow-listed keys that merely look like
// credentials to kedaCredentialKey. None of the current keys do; the list
// exists so a future one (and the brief's example) is explicit.
var kedaCredentialKeyAllow = map[string]bool{"consumerGroup": true}

// KEDATriggerTypeNames lists the accepted trigger types, sorted.
func KEDATriggerTypeNames() []string {
	out := make([]string, 0, len(kedaTriggerTypes))
	for k := range kedaTriggerTypes {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func hasControlChar(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return true
		}
	}
	return false
}

// valueHasUserinfo catches a URL with userinfo anywhere in a setting value.
func valueHasUserinfo(v string) bool {
	for _, f := range strings.FieldsFunc(v, func(r rune) bool { return r == ' ' || r == ',' }) {
		if i := strings.Index(f, "://"); i >= 0 {
			rest := f[i+3:]
			if j := strings.IndexAny(rest, "/?#"); j >= 0 {
				rest = rest[:j]
			}
			if strings.Contains(rest, "@") {
				return true
			}
		}
	}
	return false
}

func buildKEDATrigger(i int, t ScaledObjectTriggerRequest) (map[string]any, error) {
	field := fmt.Sprintf("triggers[%d]", i)
	spec, ok := kedaTriggerTypes[t.Type]
	if !ok {
		if t.Type == "" {
			return nil, hpaBadRequest("%s.type is required: allowed types are %s", field, strings.Join(KEDATriggerTypeNames(), ", "))
		}
		// The value is not echoed: it is free client input.
		return nil, hpaBadRequest("%s: unknown trigger type: allowed types are %s", field, strings.Join(KEDATriggerTypeNames(), ", "))
	}
	field = fmt.Sprintf("%s (%s)", field, t.Type)
	if t.Name != "" && (len(t.Name) > 63 || !hpaDNSLabelRE.MatchString(t.Name)) {
		return nil, hpaBadRequest("%s: name must be a DNS-1123 label", field)
	}
	if len(t.Settings) > 16 {
		return nil, hpaBadRequest("%s: too many settings", field)
	}
	keys := make([]string, 0, len(t.Settings))
	for k := range t.Settings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := t.Settings[k]
		if !spec.has(k) && kedaCredentialKey.MatchString(k) && !kedaCredentialKeyAllow[k] {
			return nil, hpaBadRequest("%s: setting %q looks like a credential; credentials are never accepted in trigger settings: create a TriggerAuthentication and pass its name as authenticationRef",
				field, safeEcho(k))
		}
		if !spec.has(k) {
			msg := fmt.Sprintf("%s: unknown setting %q: allowed settings are %s", field, safeEcho(k), strings.Join(spec.allowed(), ", "))
			if spec.note != "" {
				msg += "; " + spec.note
			}
			return nil, hpaBadRequest("%s", msg)
		}
		if v == "" {
			return nil, hpaBadRequest("%s: setting %s must not be empty", field, k)
		}
		if len(v) > kedaMaxSettingLen || hasControlChar(v) {
			return nil, hpaBadRequest("%s: setting %s must be at most %d characters without control characters", field, k, kedaMaxSettingLen)
		}
		if valueHasUserinfo(v) {
			return nil, hpaBadRequest("%s: setting %s must not contain credentials in a URL; use authenticationRef", field, k)
		}
		if c := spec.check[k]; c != nil {
			if err := c(v); err != nil {
				return nil, hpaBadRequest("%s: setting %s %v", field, k, err)
			}
		}
	}
	for _, k := range spec.required {
		if _, ok := t.Settings[k]; !ok {
			return nil, hpaBadRequest("%s: setting %s is required (required: %s)", field, k, strings.Join(spec.required, ", "))
		}
	}
	switch {
	case spec.auth == "forbidden" && t.AuthenticationRef != "":
		return nil, hpaBadRequest("%s: %s triggers take no authenticationRef", field, t.Type)
	case spec.auth == "required" && t.AuthenticationRef == "":
		return nil, hpaBadRequest("%s: authenticationRef is required (%s)", field, spec.note)
	}
	if t.AuthenticationRef != "" && (len(t.AuthenticationRef) > 253 || !hpaDNSSubdomainRE.MatchString(t.AuthenticationRef)) {
		return nil, hpaBadRequest("%s: authenticationRef must be the name of a TriggerAuthentication (DNS-1123)", field)
	}

	md := map[string]any{}
	out := map[string]any{"type": t.Type}
	for _, k := range keys {
		if spec.metricType && k == "type" {
			out["metricType"] = t.Settings[k]
			continue
		}
		md[k] = t.Settings[k]
	}
	if spec.metricType && t.Settings["type"] == "Utilization" {
		if n, err := strconv.ParseInt(t.Settings["value"], 10, 64); err != nil || n < 1 || n > 1000 {
			return nil, hpaBadRequest("%s: setting value must be a percentage (1-1000) when type is Utilization", field)
		}
	}
	out["metadata"] = md
	if t.Name != "" {
		out["name"] = t.Name
	}
	if t.AuthenticationRef != "" {
		out["authenticationRef"] = map[string]any{"name": t.AuthenticationRef}
	}
	return out, nil
}

// buildScaledObject validates req and renders the ScaledObject.
func buildScaledObject(req ScaledObjectCreateRequest) (autoscalerPlan, error) {
	if err := checkAutoscalerTarget(req.Namespace, req.Name, req.TargetKind, req.TargetName, kedaTargetKinds, "Deployment or StatefulSet"); err != nil {
		return autoscalerPlan{}, err
	}
	if len(req.Triggers) == 0 {
		return autoscalerPlan{}, hpaBadRequest("at least one trigger is required: allowed types are %s", strings.Join(KEDATriggerTypeNames(), ", "))
	}
	if len(req.Triggers) > kedaMaxTriggers {
		return autoscalerPlan{}, hpaBadRequest("at most %d triggers are allowed", kedaMaxTriggers)
	}
	triggers := make([]any, 0, len(req.Triggers))
	names := map[string]bool{}
	authSeen := map[string]bool{}
	var authRefs []string
	eventTrigger := false
	for i, t := range req.Triggers {
		tr, err := buildKEDATrigger(i, t)
		if err != nil {
			return autoscalerPlan{}, err
		}
		if t.Name != "" {
			if names[t.Name] {
				return autoscalerPlan{}, hpaBadRequest("trigger name %q is used twice", t.Name)
			}
			names[t.Name] = true
		}
		if t.Type != "cpu" && t.Type != "memory" {
			eventTrigger = true
		}
		if t.AuthenticationRef != "" && !authSeen[t.AuthenticationRef] {
			authSeen[t.AuthenticationRef] = true
			authRefs = append(authRefs, t.AuthenticationRef)
		}
		triggers = append(triggers, tr)
	}

	if req.MaxReplicas < 1 || req.MaxReplicas > hpaMaxReplicasLimit {
		return autoscalerPlan{}, hpaBadRequest("maxReplicas must be between 1 and %d", hpaMaxReplicasLimit)
	}
	var minReplicas int32
	switch {
	case req.MinReplicas != nil:
		minReplicas = *req.MinReplicas
	case eventTrigger:
		minReplicas = 0
	default:
		minReplicas = 1
	}
	if minReplicas < 0 || minReplicas > req.MaxReplicas {
		return autoscalerPlan{}, hpaBadRequest("minReplicas must be between 0 and maxReplicas (%d)", req.MaxReplicas)
	}
	if minReplicas == 0 && !eventTrigger {
		return autoscalerPlan{}, hpaBadRequest("minReplicas 0 (scale to zero) needs at least one trigger that is not cpu or memory: " +
			"cpu and memory are measured on running pods, so KEDA cannot wake a workload from zero on them alone; set minReplicas to at least 1 or add an event trigger")
	}
	polling, cooldown := req.PollingInterval, req.CooldownPeriod
	if polling == 0 {
		polling = kedaDefaultPolling
	}
	if cooldown == 0 {
		cooldown = kedaDefaultCooldown
	}
	if polling < 1 || polling > kedaMaxPolling {
		return autoscalerPlan{}, hpaBadRequest("pollingInterval must be between 1 and %d seconds", kedaMaxPolling)
	}
	if cooldown < 1 || cooldown > kedaMaxCooldown {
		return autoscalerPlan{}, hpaBadRequest("cooldownPeriod must be between 1 and %d seconds", kedaMaxCooldown)
	}

	spec := map[string]any{
		"scaleTargetRef":  map[string]any{"kind": req.TargetKind, "name": req.TargetName},
		"minReplicaCount": int64(minReplicas),
		"maxReplicaCount": int64(req.MaxReplicas),
		"pollingInterval": int64(polling),
		"cooldownPeriod":  int64(cooldown),
		"triggers":        triggers,
	}
	return autoscalerPlan{
		Object: autoscalerObject{
			Group: "keda.sh", Version: "v1alpha1", Resource: "scaledobjects", Kind: "ScaledObject",
			Namespace: req.Namespace, Name: req.Name,
			Obj: map[string]any{
				"kind":     "ScaledObject",
				"metadata": autoscalerMeta(req.Namespace, req.Name),
				"spec":     spec,
			},
		},
		Warnings:   []string{},
		AuthRefs:   authRefs,
		TargetKind: req.TargetKind, TargetName: req.TargetName,
	}, nil
}

// PreviewScaledObject validates req and renders what CreateScaledObject would
// create. It never talks to a cluster: authenticationRef existence and the
// HPA conflict warning are only checked by create.
func PreviewScaledObject(req ScaledObjectCreateRequest) (AutoscalerPreview, error) {
	plan, err := buildScaledObject(req)
	if err != nil {
		return AutoscalerPreview{}, err
	}
	return plan.preview()
}

// scaledObjectSummaryFromPlan is the list entry for a just-created object
// (demo store), mirroring scaledObjectFromUnstructured.
func scaledObjectSummaryFromPlan(req ScaledObjectCreateRequest, plan autoscalerPlan) ScaledObjectSummary {
	spec := plan.Object.Obj["spec"].(map[string]any)
	so := ScaledObjectSummary{
		Namespace: req.Namespace, Name: req.Name, TargetKind: req.TargetKind, TargetName: req.TargetName,
		MinReplicas: int32(spec["minReplicaCount"].(int64)), MaxReplicas: req.MaxReplicas,
		Triggers: []ScaledObjectTrigger{}, Conditions: []AutoscalerCondition{}, Age: "now",
	}
	for _, t := range req.Triggers {
		md := map[string]any{}
		for k, v := range t.Settings {
			md[k] = v
		}
		so.Triggers = append(so.Triggers, ScaledObjectTrigger{Type: t.Type, Name: kedaTriggerName(t.Name, md)})
	}
	return so
}

// vpaSummaryFromPlan is the list entry for a just-created VPA (demo store).
func vpaSummaryFromPlan(req VPACreateRequest, plan autoscalerPlan) VPASummary {
	mode := req.UpdateMode
	if mode == "" {
		mode = "Off"
	}
	return VPASummary{
		Namespace: req.Namespace, Name: req.Name, TargetKind: req.TargetKind, TargetName: req.TargetName,
		UpdateMode: mode, Containers: []VPAContainerRecommendation{}, Conditions: []AutoscalerCondition{}, Age: "now",
	}
}
