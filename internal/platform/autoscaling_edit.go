package platform

import (
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/yaml"
)

// VerticalPodAutoscaler and KEDA ScaledObject editing in place: reading the
// typed fields back out of a live object, and turning an edited typed request
// into a precise patch.
//
// This file is pure (no cluster access) and compiles in both build modes. The
// design is the GitOps edit's (gitops_edit.go): the request is the creation
// body plus the resourceVersion read from GET .../edit, it is validated by
// building the object with buildVPA / buildScaledObject, and only the fields
// the typed model manages are written, as a JSON Patch whose first op tests
// metadata.resourceVersion. Labels, annotations, finalizers, status and spec
// keys the model does not manage are left exactly as they are.
//
// Objects that use something the typed model cannot represent are reported
// with editable:false and the reasons; editing them is a 400. Trigger setting
// values that look like credentials are never returned (the trigger is then
// non-editable), and reasons name keys, never values.

// autoscalerIdentityFields are the typed fields that can never change.
var autoscalerIdentityFields = []string{"namespace", "name", "targetKind", "targetName"}

// VPAEditable is the response of GET /api/v1/vpas/{namespace}/{name}/edit:
// the creation body's fields with the object's current values.
type VPAEditable struct {
	Namespace           string                      `json:"namespace"`
	Name                string                      `json:"name"`
	TargetKind          string                      `json:"targetKind"`
	TargetName          string                      `json:"targetName"`
	UpdateMode          string                      `json:"updateMode"`
	ControlledResources []string                    `json:"controlledResources"`
	Containers          []VPAContainerPolicyRequest `json:"containers"`
	// ResourceVersion must be echoed back in the edit body; a mismatch is a 409.
	ResourceVersion string `json:"resourceVersion"`
	// Editable is false when the object uses something the typed model cannot
	// represent; Reasons says what.
	Editable bool     `json:"editable"`
	Reasons  []string `json:"reasons"`
	// ReadOnly lists the typed fields that cannot be changed (the identity).
	ReadOnly []string `json:"readOnly"`
}

// VPAEditRequest is the body of POST .../edit/preview and PUT
// /api/v1/vpas/{namespace}/{name}. Identity fields may be echoed but not
// changed. An omitted updateMode / controlledResources / containers (JSON
// absent or null) keeps the current value; "containers": [] removes every
// container policy.
type VPAEditRequest struct {
	VPACreateRequest
	ResourceVersion string `json:"resourceVersion"`
}

// ScaledObjectEditable is the response of GET
// /api/v1/scaledobjects/{namespace}/{name}/edit.
type ScaledObjectEditable struct {
	Namespace       string                       `json:"namespace"`
	Name            string                       `json:"name"`
	TargetKind      string                       `json:"targetKind"`
	TargetName      string                       `json:"targetName"`
	MinReplicas     int32                        `json:"minReplicas"`
	MaxReplicas     int32                        `json:"maxReplicas"`
	PollingInterval int32                        `json:"pollingInterval"`
	CooldownPeriod  int32                        `json:"cooldownPeriod"`
	Triggers        []ScaledObjectTriggerRequest `json:"triggers"`
	ResourceVersion string                       `json:"resourceVersion"`
	Editable        bool                         `json:"editable"`
	Reasons         []string                     `json:"reasons"`
	ReadOnly        []string                     `json:"readOnly"`
}

// ScaledObjectEditRequest is the body of POST .../edit/preview and PUT
// /api/v1/scaledobjects/{namespace}/{name}. An omitted minReplicas,
// maxReplicas (0), pollingInterval (0), cooldownPeriod (0) or triggers keeps
// the current value.
type ScaledObjectEditRequest struct {
	ScaledObjectCreateRequest
	ResourceVersion string `json:"resourceVersion"`
}

// AutoscalerEditPreview is the response of POST .../edit/preview. Changes has
// the same shape as the GitOps edit's.
type AutoscalerEditPreview struct {
	Changes  []GitOpsEditChange `json:"changes"`
	YAML     string             `json:"yaml"`
	Warnings []string           `json:"warnings"`
	Noop     bool               `json:"noop"`
}

// AutoscalerEditResult is the response of PUT .../{namespace}/{name}.
type AutoscalerEditResult struct {
	OK              bool     `json:"ok"`
	Noop            bool     `json:"noop"`
	Changed         []string `json:"changed"`
	Warnings        []string `json:"warnings"`
	ResourceVersion string   `json:"resourceVersion"`
}

// autoscalerEditPlan is the computed edit.
type autoscalerEditPlan struct {
	Preview AutoscalerEditPreview
	Patch   []byte         // JSON Patch; nil when nothing changes
	After   map[string]any // the object after the change
	Fields  []string       // changed field names (audit, PUT response)
	Build   autoscalerPlan // the validated typed object (HPA checks, auth refs)
	RV      string
}

func (p autoscalerEditPlan) result(rv string) AutoscalerEditResult {
	return AutoscalerEditResult{OK: true, Noop: p.Preview.Noop, Changed: nonNilStrings(p.Fields),
		Warnings: nonNilStrings(p.Preview.Warnings), ResourceVersion: rv}
}

const autoscalerConflictText = "the object changed since you opened it: reload it and apply your edit again"

func autoscalerEditConflict() error {
	return NewStatusError(http.StatusConflict, fmt.Errorf("%s", autoscalerConflictText))
}

type editBlocker struct {
	reasons []string
}

func (b *editBlocker) add(format string, a ...any) {
	b.reasons = append(b.reasons, fmt.Sprintf(format, a...))
}

func (b *editBlocker) list() []string { return nonNilStrings(b.reasons) }

// editQuantityText reads a live quantity (string or number) as text.
func editQuantityText(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case int64:
		return strconv.FormatInt(t, 10), true
	case int:
		return strconv.Itoa(t), true
	case int32:
		return strconv.FormatInt(int64(t), 10), true
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), true
	}
	return "", false
}

// editInt reads a live integer field; def when absent, ok false when it is
// not a whole number.
func editInt(v any, def int32) (int32, bool) {
	switch t := v.(type) {
	case nil:
		return def, true
	case int64:
		return int32(t), true
	case int:
		return int32(t), true
	case int32:
		return t, true
	case float64:
		if t == float64(int32(t)) {
			return int32(t), true
		}
	}
	return 0, false
}

func canonicalQuantity(s string) string {
	if q, err := resource.ParseQuantity(s); err == nil {
		return q.String()
	}
	return s
}

func checkEditIdentity(kind string, got, want []string) error {
	for i, f := range autoscalerIdentityFields {
		if got[i] != "" && got[i] != want[i] {
			return hpaBadRequest("%s cannot be changed (it is %q); create a new %s instead", f, want[i], kind)
		}
	}
	return nil
}

func checkEditResourceVersion(got, want string) error {
	if got == "" {
		return hpaBadRequest("resourceVersion is required: send back the resourceVersion from GET .../edit")
	}
	if got != want {
		return autoscalerEditConflict()
	}
	return nil
}

// autoscalerEditYAML renders the object after the change without
// managedFields and the kubectl last-applied annotation (which can hold an
// older copy of the spec).
func autoscalerEditYAML(obj map[string]any) string {
	cp, _ := jsonNormalize(obj).(map[string]any)
	if meta, ok := cp["metadata"].(map[string]any); ok {
		delete(meta, "managedFields")
		if ann, ok := meta["annotations"].(map[string]any); ok {
			delete(ann, "kubectl.kubernetes.io/last-applied-configuration")
			if len(ann) == 0 {
				delete(meta, "annotations")
			}
		}
	}
	b, err := yaml.Marshal(cp)
	if err != nil {
		return ""
	}
	return string(b)
}

// finish turns the editor state into the plan.
func finishAutoscalerEdit(o *gitOpsEditObj, built autoscalerPlan) autoscalerEditPlan {
	plan := autoscalerEditPlan{Build: built, RV: o.rv}
	plan.Preview.Changes = append([]GitOpsEditChange{}, o.changes...)
	for _, c := range o.changes {
		plan.Fields = append(plan.Fields, c.Field)
	}
	plan.Fields = nonNilStrings(plan.Fields)
	plan.Preview.YAML = autoscalerEditYAML(o.cur)
	plan.Preview.Warnings = nonNilStrings(append([]string{}, built.Warnings...))
	plan.Preview.Noop = len(o.changes) == 0
	plan.After = o.cur
	if o.changed() {
		plan.Patch = o.patch()
	}
	return plan
}

// setReplacing applies set and replaces the change it recorded (if any) with
// the given, finer-grained display changes.
func (o *gitOpsEditObj) setReplacing(want any, changes []GitOpsEditChange, fields ...string) {
	n := len(o.changes)
	o.set(want, fields...)
	if len(o.changes) > n {
		o.changes = append(o.changes[:n], changes...)
	}
}

// --- VerticalPodAutoscaler ---

var (
	vpaTargetRefKeys = map[string]bool{"apiVersion": true, "kind": true, "name": true}
	vpaPolicyKeys    = map[string]bool{"containerName": true, "controlledResources": true, "minAllowed": true, "maxAllowed": true}
)

// vpaPolicyNorm is one container policy reduced to what the model manages.
type vpaPolicyNorm struct {
	Controlled                     []string
	MinCPU, MaxCPU, MinMem, MaxMem string
}

func (p vpaPolicyNorm) canonical() vpaPolicyNorm {
	c := append([]string{}, p.Controlled...)
	sort.Strings(c)
	q := func(s string) string {
		if s == "" {
			return ""
		}
		return canonicalQuantity(s)
	}
	return vpaPolicyNorm{Controlled: c, MinCPU: q(p.MinCPU), MaxCPU: q(p.MaxCPU), MinMem: q(p.MinMem), MaxMem: q(p.MaxMem)}
}

func (p vpaPolicyNorm) String() string {
	parts := []string{"controlledResources=" + strings.Join(p.Controlled, ",")}
	for _, kv := range [][2]string{{"minAllowed.cpu", p.MinCPU}, {"minAllowed.memory", p.MinMem}, {"maxAllowed.cpu", p.MaxCPU}, {"maxAllowed.memory", p.MaxMem}} {
		if kv[1] != "" {
			parts = append(parts, kv[0]+"="+kv[1])
		}
	}
	return strings.Join(parts, " ")
}

// readVPAPolicy reads one live container policy; problems go to b.
func readVPAPolicy(i int, raw any, b *editBlocker) (string, vpaPolicyNorm, bool) {
	m, ok := raw.(map[string]any)
	if !ok {
		b.add("spec.resourcePolicy.containerPolicies[%d] is not an object", i)
		return "", vpaPolicyNorm{}, false
	}
	name, _ := m["containerName"].(string)
	label := fmt.Sprintf("container policy %q", safeEcho(name))
	if name == "*" {
		label = `container policy "*"`
	}
	if name == "" {
		b.add("spec.resourcePolicy.containerPolicies[%d] has no containerName", i)
		return "", vpaPolicyNorm{}, false
	}
	keys := sortedKeys(m)
	good := true
	for _, k := range keys {
		switch {
		case vpaPolicyKeys[k]:
		case k == "mode":
			b.add("%s sets mode (the form cannot turn a container's recommendations off)", label)
			good = false
		case k == "controlledValues":
			b.add("%s sets controlledValues (the form always controls requests and limits)", label)
			good = false
		default:
			b.add("%s uses %s, which the form cannot represent", label, safeEcho(k))
			good = false
		}
	}
	p := vpaPolicyNorm{Controlled: []string{"cpu", "memory"}}
	if raw, ok := m["controlledResources"]; ok {
		list, isList := raw.([]any)
		if !isList {
			b.add("%s has an invalid controlledResources", label)
			return name, p, false
		}
		p.Controlled = []string{}
		if len(list) == 0 {
			b.add("%s controls no resources", label)
			good = false
		}
		for _, r := range list {
			s, _ := r.(string)
			if s != "cpu" && s != "memory" {
				b.add("%s controls %s; only cpu and memory can be edited here", label, safeEcho(s))
				good = false
				continue
			}
			p.Controlled = append(p.Controlled, s)
		}
	}
	for _, side := range []string{"minAllowed", "maxAllowed"} {
		raw, ok := m[side]
		if !ok || raw == nil {
			continue
		}
		bounds, isMap := raw.(map[string]any)
		if !isMap {
			b.add("%s has an invalid %s", label, side)
			good = false
			continue
		}
		for _, k := range sortedKeys(bounds) {
			if k != "cpu" && k != "memory" {
				b.add("%s sets %s.%s; only cpu and memory bounds can be edited here", label, side, safeEcho(k))
				good = false
				continue
			}
			v, ok := editQuantityText(bounds[k])
			if !ok {
				b.add("%s has an invalid %s.%s", label, side, k)
				good = false
				continue
			}
			switch side + "." + k {
			case "minAllowed.cpu":
				p.MinCPU = v
			case "minAllowed.memory":
				p.MinMem = v
			case "maxAllowed.cpu":
				p.MaxCPU = v
			case "maxAllowed.memory":
				p.MaxMem = v
			}
		}
	}
	return name, p, good
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// vpaPolicies reads a containerPolicies list into name → policy, keeping order.
func vpaPolicies(raw any, b *editBlocker) ([]string, map[string]vpaPolicyNorm) {
	list, _ := raw.([]any)
	names := []string{}
	out := map[string]vpaPolicyNorm{}
	for i, p := range list {
		name, pol, _ := readVPAPolicy(i, p, b)
		if name == "" {
			continue
		}
		if _, dup := out[name]; dup {
			b.add("container policy %q is listed twice", safeEcho(name))
			continue
		}
		names = append(names, name)
		out[name] = pol
	}
	return names, out
}

// readVPAEditable extracts the typed fields from a live VerticalPodAutoscaler.
func readVPAEditable(obj map[string]any) VPAEditable {
	ed := VPAEditable{
		Namespace: nestedString(obj, "metadata", "namespace"), Name: nestedString(obj, "metadata", "name"),
		ResourceVersion:     nestedString(obj, "metadata", "resourceVersion"),
		ControlledResources: []string{"cpu", "memory"}, Containers: []VPAContainerPolicyRequest{},
		ReadOnly: append([]string{}, autoscalerIdentityFields...),
	}
	b := &editBlocker{}
	spec, _ := nestedMap(obj, "spec")

	if tr, ok := spec["targetRef"].(map[string]any); !ok {
		b.add("has no spec.targetRef")
	} else {
		for _, k := range sortedKeys(tr) {
			if !vpaTargetRefKeys[k] {
				b.add("spec.targetRef.%s is not supported here", safeEcho(k))
			}
		}
		ed.TargetKind, _ = tr["kind"].(string)
		ed.TargetName, _ = tr["name"].(string)
		switch av, _ := tr["apiVersion"].(string); {
		case av == "":
			b.add("spec.targetRef has no apiVersion; only apps/v1 workloads can be edited here")
		case av != "apps/v1":
			b.add("spec.targetRef.apiVersion is %s; only apps/v1 workloads can be edited here", safeEcho(av))
		}
		if !vpaTargetKinds[ed.TargetKind] {
			b.add("targets a %s; only a Deployment, StatefulSet, DaemonSet or ReplicaSet can be edited here", safeEcho(ed.TargetKind))
		}
	}
	if nonEmpty(spec["selector"]) {
		b.add("uses spec.selector instead of a single spec.targetRef")
	}
	if nonEmpty(spec["recommenders"]) {
		b.add("uses spec.recommenders (a non-default recommender)")
	}
	up, _ := spec["updatePolicy"].(map[string]any)
	if nonEmpty(up["evictionRequirements"]) {
		b.add("uses spec.updatePolicy.evictionRequirements")
	}
	ed.UpdateMode, _ = up["updateMode"].(string)
	if ed.UpdateMode == "" {
		ed.UpdateMode = "Auto" // the VPA default when unset
	}
	if !containsString(vpaUpdateModes, ed.UpdateMode) {
		b.add("uses updateMode %s; the form supports Off, Initial, Recreate and Auto", safeEcho(ed.UpdateMode))
	}

	rp, _ := spec["resourcePolicy"].(map[string]any)
	for _, k := range sortedKeys(rp) {
		if k != "containerPolicies" {
			b.add("uses spec.resourcePolicy.%s", safeEcho(k))
		}
	}
	names, pols := vpaPolicies(rp["containerPolicies"], b)
	var controlled []string
	for i, n := range names {
		if i == 0 {
			controlled = pols[n].Controlled
			continue
		}
		if !reflect.DeepEqual(pols[n].canonical().Controlled, pols[names[0]].canonical().Controlled) {
			b.add("its container policies control different resources; the form sets one list for every container")
			break
		}
	}
	if len(controlled) > 0 {
		ed.ControlledResources = append([]string{}, controlled...)
	}
	// A lone "*" policy without bounds only carries a controlledResources
	// restriction; creation adds it by itself, so it is not a container entry.
	carrier := len(names) == 1 && names[0] == "*" && len(controlled) < 2 &&
		pols["*"].MinCPU == "" && pols["*"].MaxCPU == "" && pols["*"].MinMem == "" && pols["*"].MaxMem == ""
	if !carrier {
		for _, n := range names {
			p := pols[n]
			ed.Containers = append(ed.Containers, VPAContainerPolicyRequest{Name: n, MinCPU: p.MinCPU, MaxCPU: p.MaxCPU, MinMemory: p.MinMem, MaxMemory: p.MaxMem})
		}
	}
	ed.Reasons = b.list()
	ed.Editable = len(ed.Reasons) == 0
	return ed
}

// planVPAEdit validates req against the live object and computes the patch.
func planVPAEdit(obj map[string]any, req VPAEditRequest) (autoscalerEditPlan, error) {
	ed := readVPAEditable(obj)
	if !ed.Editable {
		return autoscalerEditPlan{}, hpaBadRequest("VerticalPodAutoscaler %s/%s cannot be edited here: %s", ed.Namespace, ed.Name, strings.Join(ed.Reasons, "; "))
	}
	if err := checkEditIdentity("VerticalPodAutoscaler",
		[]string{req.Namespace, req.Name, req.TargetKind, req.TargetName},
		[]string{ed.Namespace, ed.Name, ed.TargetKind, ed.TargetName}); err != nil {
		return autoscalerEditPlan{}, err
	}
	if err := checkEditResourceVersion(req.ResourceVersion, ed.ResourceVersion); err != nil {
		return autoscalerEditPlan{}, err
	}
	cr := req.VPACreateRequest
	cr.Namespace, cr.Name, cr.TargetKind, cr.TargetName = ed.Namespace, ed.Name, ed.TargetKind, ed.TargetName
	if cr.UpdateMode == "" {
		cr.UpdateMode = ed.UpdateMode
	}
	if cr.ControlledResources == nil {
		cr.ControlledResources = ed.ControlledResources
	}
	if cr.Containers == nil {
		cr.Containers = ed.Containers
	}
	built, err := buildVPA(cr)
	if err != nil {
		return autoscalerEditPlan{}, err
	}
	rs, _ := built.Object.Obj["spec"].(map[string]any)

	o := (&gitOpsEditor{}).begin("VerticalPodAutoscaler", obj)
	if want := nestedString(rs, "updatePolicy", "updateMode"); !(nestedValue(o.cur, "spec", "updatePolicy", "updateMode") == nil && want == "Auto") {
		o.set(want, "spec", "updatePolicy", "updateMode")
	}

	wantRaw := nestedValue(rs, "resourcePolicy", "containerPolicies")
	ignore := &editBlocker{}
	liveNames, live := vpaPolicies(nestedValue(o.cur, "spec", "resourcePolicy", "containerPolicies"), ignore)
	wantNames, want := vpaPolicies(wantRaw, ignore)
	var changes []GitOpsEditChange
	seen := map[string]bool{}
	for _, n := range append(append([]string{}, wantNames...), liveNames...) {
		if seen[n] {
			continue
		}
		seen[n] = true
		l, lok := live[n]
		w, wok := want[n]
		if lok && wok && reflect.DeepEqual(l.canonical(), w.canonical()) {
			continue
		}
		c := GitOpsEditChange{Object: o.label, Field: "spec.resourcePolicy.containerPolicies[" + n + "]"}
		if lok {
			c.From = l.String()
		}
		if wok {
			c.To = w.String()
		}
		changes = append(changes, c)
	}
	if len(changes) > 0 {
		if list, _ := wantRaw.([]any); len(list) == 0 {
			o.setReplacing(nil, changes, "spec", "resourcePolicy")
		} else {
			o.setReplacing(wantRaw, changes, "spec", "resourcePolicy", "containerPolicies")
		}
	}
	return finishAutoscalerEdit(o, built), nil
}

// --- KEDA ScaledObject ---

var (
	kedaScaleTargetKeys = map[string]bool{"apiVersion": true, "kind": true, "name": true}
	kedaTriggerKeys     = map[string]bool{"type": true, "name": true, "metadata": true, "authenticationRef": true, "metricType": true}
)

// kedaCredentialLike reports whether a live setting value of an allow-listed
// key looks like it carries a credential (the create checks would refuse it
// for that reason).
func kedaCredentialLike(spec kedaTriggerType, k, v string) bool {
	if valueHasUserinfo(v) {
		return true
	}
	if c := spec.check[k]; c != nil {
		if err := c(v); err != nil && strings.Contains(err.Error(), "credentials") {
			return true
		}
	}
	return false
}

// readKEDATrigger reads one live trigger into the typed form. Values of
// unknown or credential-like settings are never copied.
func readKEDATrigger(i int, raw any, b *editBlocker) ScaledObjectTriggerRequest {
	out := ScaledObjectTriggerRequest{Settings: map[string]string{}}
	m, ok := raw.(map[string]any)
	if !ok {
		b.add("triggers[%d] is not an object", i)
		return out
	}
	out.Type, _ = m["type"].(string)
	out.Name, _ = m["name"].(string)
	spec, known := kedaTriggerTypes[out.Type]
	label := fmt.Sprintf("triggers[%d] (%s)", i, safeEcho(out.Type))
	if !known {
		b.add("triggers[%d] has type %s, which the form cannot edit (allowed: %s)", i, safeEcho(out.Type), strings.Join(KEDATriggerTypeNames(), ", "))
		return out
	}
	for _, k := range sortedKeys(m) {
		switch {
		case kedaTriggerKeys[k]:
		case k == "useCachedMetrics":
			if nonEmpty(m[k]) {
				b.add("%s sets useCachedMetrics", label)
			}
		default:
			b.add("%s uses %s, which the form cannot represent", label, safeEcho(k))
		}
	}
	mt, _ := m["metricType"].(string)
	if spec.metricType {
		if mt != "" {
			out.Settings["type"] = mt
		}
	} else if mt != "" {
		b.add("%s sets metricType %s; only cpu and memory triggers take a metricType here", label, safeEcho(mt))
	}
	md, isMap := m["metadata"].(map[string]any)
	if m["metadata"] != nil && !isMap {
		b.add("%s has invalid metadata", label)
	}
	for _, k := range sortedKeys(md) {
		if spec.metricType && k == "type" {
			b.add("%s uses the deprecated metadata.type; set metricType instead", label)
			continue
		}
		if !spec.has(k) {
			if kedaCredentialKey.MatchString(k) && !kedaCredentialKeyAllow[k] {
				b.add("%s has a credential-like setting %s (not shown): credentials belong in a TriggerAuthentication", label, safeEcho(k))
			} else {
				b.add("%s uses setting %s, which the form cannot represent", label, safeEcho(k))
			}
			continue
		}
		v, isStr := md[k].(string)
		switch {
		case !isStr:
			b.add("%s: setting %s is not a string", label, k)
		case kedaCredentialLike(spec, k, v):
			b.add("%s: setting %s looks like it contains a credential, so it is not shown: move the credential to a TriggerAuthentication", label, k)
		case len(v) > kedaMaxSettingLen || hasControlChar(v):
			b.add("%s: setting %s is longer than %d characters or contains control characters", label, k, kedaMaxSettingLen)
		default:
			out.Settings[k] = v
		}
	}
	if spec.metricType && mt == "" && md["type"] == nil {
		b.add("%s has no metricType", label)
	}
	if ar, ok := m["authenticationRef"].(map[string]any); ok {
		for _, k := range sortedKeys(ar) {
			if k != "name" && k != "kind" {
				b.add("%s: authenticationRef.%s is not supported here", label, safeEcho(k))
			}
		}
		switch kind, _ := ar["kind"].(string); kind {
		case "", "TriggerAuthentication":
		case "ClusterTriggerAuthentication":
			b.add("%s uses a ClusterTriggerAuthentication; only a TriggerAuthentication in the same namespace can be referenced here", label)
		default:
			b.add("%s: authenticationRef.kind %s is not supported", label, safeEcho(kind))
		}
		out.AuthenticationRef, _ = ar["name"].(string)
		if spec.auth == "forbidden" && out.AuthenticationRef != "" {
			b.add("%s has an authenticationRef, which %s triggers do not take", label, out.Type)
		}
	} else if m["authenticationRef"] != nil {
		b.add("%s has an invalid authenticationRef", label)
	}
	return out
}

// readScaledObjectEditable extracts the typed fields from a live ScaledObject.
func readScaledObjectEditable(obj map[string]any) ScaledObjectEditable {
	ed := ScaledObjectEditable{
		Namespace: nestedString(obj, "metadata", "namespace"), Name: nestedString(obj, "metadata", "name"),
		ResourceVersion: nestedString(obj, "metadata", "resourceVersion"),
		Triggers:        []ScaledObjectTriggerRequest{},
		ReadOnly:        append([]string{}, autoscalerIdentityFields...),
	}
	b := &editBlocker{}
	spec, _ := nestedMap(obj, "spec")

	if st, ok := spec["scaleTargetRef"].(map[string]any); !ok {
		b.add("has no spec.scaleTargetRef")
	} else {
		for _, k := range sortedKeys(st) {
			switch {
			case kedaScaleTargetKeys[k]:
			case k == "envSourceContainerName":
				b.add("uses spec.scaleTargetRef.envSourceContainerName")
			default:
				b.add("spec.scaleTargetRef.%s is not supported here", safeEcho(k))
			}
		}
		ed.TargetKind, _ = st["kind"].(string)
		if ed.TargetKind == "" {
			ed.TargetKind = "Deployment" // the KEDA default
		}
		ed.TargetName, _ = st["name"].(string)
		if av, _ := st["apiVersion"].(string); av != "" && av != "apps/v1" {
			b.add("spec.scaleTargetRef.apiVersion is %s; only apps/v1 workloads can be edited here", safeEcho(av))
		}
		if !kedaTargetKinds[ed.TargetKind] {
			b.add("scales a %s; only a Deployment or StatefulSet can be edited here", safeEcho(ed.TargetKind))
		}
	}
	if nonEmpty(spec["advanced"]) {
		b.add("uses spec.advanced (HPA behavior, scaling modifiers or restoreToOriginalReplicaCount)")
	}
	if nonEmpty(spec["fallback"]) {
		b.add("uses spec.fallback")
	}
	for _, f := range []struct {
		key string
		def int32
		dst *int32
	}{
		{"minReplicaCount", 0, &ed.MinReplicas}, {"maxReplicaCount", 100, &ed.MaxReplicas},
		{"pollingInterval", kedaDefaultPolling, &ed.PollingInterval}, {"cooldownPeriod", kedaDefaultCooldown, &ed.CooldownPeriod},
	} {
		v, ok := editInt(spec[f.key], f.def)
		if !ok {
			b.add("spec.%s is not a whole number", f.key)
		}
		*f.dst = v
	}
	if raw, ok := spec["triggers"]; ok {
		list, isList := raw.([]any)
		if !isList {
			b.add("spec.triggers is not a list")
		}
		for i, t := range list {
			ed.Triggers = append(ed.Triggers, readKEDATrigger(i, t, b))
		}
	}
	ed.Reasons = b.list()
	ed.Editable = len(ed.Reasons) == 0
	return ed
}

// kedaTriggerNorm reduces a trigger to the fields the model manages, for
// comparison (an explicit authenticationRef kind TriggerAuthentication equals
// none).
func kedaTriggerNorm(raw any) any {
	m, ok := jsonNormalize(raw).(map[string]any)
	if !ok {
		return raw
	}
	out := map[string]any{"type": m["type"]}
	if n, _ := m["name"].(string); n != "" {
		out["name"] = n
	}
	if mt, _ := m["metricType"].(string); mt != "" {
		out["metricType"] = mt
	}
	md, _ := m["metadata"].(map[string]any)
	if md == nil {
		md = map[string]any{}
	}
	out["metadata"] = md
	if ar, ok := m["authenticationRef"].(map[string]any); ok {
		if n, _ := ar["name"].(string); n != "" {
			out["authenticationRef"] = n
		}
	}
	return out
}

// kedaTriggerText renders a trigger on one line for the change list. Only
// called for triggers that passed the credential screen (live, editable) or
// the create validation (wanted).
func kedaTriggerText(norm any) string {
	m, ok := norm.(map[string]any)
	if !ok {
		return ""
	}
	parts := []string{fmt.Sprintf("type=%v", m["type"])}
	if n, ok := m["name"]; ok {
		parts = append(parts, fmt.Sprintf("name=%v", n))
	}
	if mt, ok := m["metricType"]; ok {
		parts = append(parts, fmt.Sprintf("metricType=%v", mt))
	}
	md, _ := m["metadata"].(map[string]any)
	for _, k := range sortedKeys(md) {
		parts = append(parts, fmt.Sprintf("%s=%v", k, md[k]))
	}
	if a, ok := m["authenticationRef"]; ok {
		parts = append(parts, fmt.Sprintf("authenticationRef=%v", a))
	}
	return strings.Join(parts, " ")
}

// planScaledObjectEdit validates req against the live object and computes the
// patch.
func planScaledObjectEdit(obj map[string]any, req ScaledObjectEditRequest) (autoscalerEditPlan, error) {
	ed := readScaledObjectEditable(obj)
	if !ed.Editable {
		return autoscalerEditPlan{}, hpaBadRequest("ScaledObject %s/%s cannot be edited here: %s", ed.Namespace, ed.Name, strings.Join(ed.Reasons, "; "))
	}
	if err := checkEditIdentity("ScaledObject",
		[]string{req.Namespace, req.Name, req.TargetKind, req.TargetName},
		[]string{ed.Namespace, ed.Name, ed.TargetKind, ed.TargetName}); err != nil {
		return autoscalerEditPlan{}, err
	}
	if err := checkEditResourceVersion(req.ResourceVersion, ed.ResourceVersion); err != nil {
		return autoscalerEditPlan{}, err
	}
	cr := req.ScaledObjectCreateRequest
	cr.Namespace, cr.Name, cr.TargetKind, cr.TargetName = ed.Namespace, ed.Name, ed.TargetKind, ed.TargetName
	if cr.MinReplicas == nil {
		v := ed.MinReplicas
		cr.MinReplicas = &v
	}
	if cr.MaxReplicas == 0 {
		cr.MaxReplicas = ed.MaxReplicas
	}
	if cr.PollingInterval == 0 {
		cr.PollingInterval = ed.PollingInterval
	}
	if cr.CooldownPeriod == 0 {
		cr.CooldownPeriod = ed.CooldownPeriod
	}
	if cr.Triggers == nil {
		cr.Triggers = ed.Triggers
	}
	built, err := buildScaledObject(cr)
	if err != nil {
		return autoscalerEditPlan{}, err
	}
	rs, _ := built.Object.Obj["spec"].(map[string]any)

	o := (&gitOpsEditor{}).begin("ScaledObject", obj)
	for _, f := range []struct {
		key string
		def int64
	}{
		{"minReplicaCount", 0}, {"maxReplicaCount", 100},
		{"pollingInterval", int64(kedaDefaultPolling)}, {"cooldownPeriod", int64(kedaDefaultCooldown)},
	} {
		want := rs[f.key]
		if nestedValue(o.cur, "spec", f.key) == nil && want == f.def {
			continue // unset means the KEDA default, which is what is wanted
		}
		o.set(want, "spec", f.key)
	}

	liveList, _ := nestedValue(o.cur, "spec", "triggers").([]any)
	wantList, _ := rs["triggers"].([]any)
	var changes []GitOpsEditChange
	for i := 0; i < len(liveList) || i < len(wantList); i++ {
		var l, w any
		if i < len(liveList) {
			l = kedaTriggerNorm(liveList[i])
		}
		if i < len(wantList) {
			w = kedaTriggerNorm(wantList[i])
		}
		if reflect.DeepEqual(l, w) {
			continue
		}
		changes = append(changes, GitOpsEditChange{Object: o.label, Field: fmt.Sprintf("spec.triggers[%d]", i),
			From: kedaTriggerText(l), To: kedaTriggerText(w)})
	}
	if len(changes) > 0 {
		o.setReplacing(rs["triggers"], changes, "spec", "triggers")
	}
	return finishAutoscalerEdit(o, built), nil
}

// demoBumpResourceVersion returns rv+1 (the demo stores' fake versions).
func demoBumpResourceVersion(rv string) string {
	n, _ := strconv.Atoi(rv)
	return strconv.Itoa(n + 1)
}
