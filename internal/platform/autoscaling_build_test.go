package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func i32p(v int32) *int32 { return &v }

func baseVPA() VPACreateRequest {
	return VPACreateRequest{
		Namespace: "commerce", Name: "checkout-vpa", TargetKind: "Deployment", TargetName: "checkout",
		ControlledResources: []string{"cpu", "memory"},
		Containers:          []VPAContainerPolicyRequest{{Name: "app", MinCPU: "50m", MaxCPU: "2", MinMemory: "64Mi", MaxMemory: "2Gi"}},
	}
}

func baseSO() ScaledObjectCreateRequest {
	return ScaledObjectCreateRequest{
		Namespace: "payments", Name: "payments-api-scaler", TargetKind: "Deployment", TargetName: "payments-api",
		MinReplicas: i32p(0), MaxReplicas: 20, PollingInterval: 30, CooldownPeriod: 300,
		Triggers: []ScaledObjectTriggerRequest{
			{Type: "cron", Name: "business-hours", Settings: map[string]string{"timezone": "Europe/Berlin", "start": "0 8 * * 1-5", "end": "0 18 * * 1-5", "desiredReplicas": "5"}},
			{Type: "prometheus", Settings: map[string]string{"serverAddress": "http://prometheus.monitoring:9090", "query": "sum(rate(http_requests_total[2m]))", "threshold": "100"}, AuthenticationRef: "prom-auth"},
		},
	}
}

const goldenVPA = `apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata:
  labels:
    app.kubernetes.io/managed-by: zyvor-axiom
  name: checkout-vpa
  namespace: commerce
spec:
  resourcePolicy:
    containerPolicies:
    - containerName: app
      controlledResources:
      - cpu
      - memory
      maxAllowed:
        cpu: "2"
        memory: 2Gi
      minAllowed:
        cpu: 50m
        memory: 64Mi
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: checkout
  updatePolicy:
    updateMode: "Off"
`

const goldenVPACPUOnly = `apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata:
  labels:
    app.kubernetes.io/managed-by: zyvor-axiom
  name: redis-vpa
  namespace: data
spec:
  resourcePolicy:
    containerPolicies:
    - containerName: '*'
      controlledResources:
      - cpu
  targetRef:
    apiVersion: apps/v1
    kind: StatefulSet
    name: redis
  updatePolicy:
    updateMode: Auto
`

const goldenScaledObject = `apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  labels:
    app.kubernetes.io/managed-by: zyvor-axiom
  name: payments-api-scaler
  namespace: payments
spec:
  cooldownPeriod: 300
  maxReplicaCount: 20
  minReplicaCount: 0
  pollingInterval: 30
  scaleTargetRef:
    kind: Deployment
    name: payments-api
  triggers:
  - metadata:
      desiredReplicas: "5"
      end: 0 18 * * 1-5
      start: 0 8 * * 1-5
      timezone: Europe/Berlin
    name: business-hours
    type: cron
  - authenticationRef:
      name: prom-auth
    metadata:
      query: sum(rate(http_requests_total[2m]))
      serverAddress: http://prometheus.monitoring:9090
      threshold: "100"
    type: prometheus
`

func TestPreviewVPAGolden(t *testing.T) {
	p, err := PreviewVPA(baseVPA())
	if err != nil {
		t.Fatal(err)
	}
	if p.YAML != goldenVPA {
		t.Fatalf("yaml:\n%s\nwant:\n%s", p.YAML, goldenVPA)
	}
	if len(p.Objects) != 1 || p.Objects[0] != (AutoscalerObjectRef{APIVersion: "autoscaling.k8s.io/v1", Kind: "VerticalPodAutoscaler", Namespace: "commerce", Name: "checkout-vpa"}) {
		t.Fatalf("objects: %+v", p.Objects)
	}
	if p.Warnings == nil || len(p.Warnings) != 0 {
		t.Fatalf("Off must not warn: %#v", p.Warnings)
	}

	// Controlled resources restricted, no container bounds: a "*" policy.
	p, err = PreviewVPA(VPACreateRequest{Namespace: "data", Name: "redis-vpa", TargetKind: "StatefulSet", TargetName: "redis", UpdateMode: "Auto", ControlledResources: []string{"cpu"}})
	if err != nil {
		t.Fatal(err)
	}
	if p.YAML != goldenVPACPUOnly {
		t.Fatalf("yaml:\n%s\nwant:\n%s", p.YAML, goldenVPACPUOnly)
	}
	if len(p.Warnings) != 1 || !strings.Contains(p.Warnings[0], "pods may be restarted to apply new requests") {
		t.Fatalf("Auto must warn: %v", p.Warnings)
	}

	// Defaults: no containers, both resources -> no resourcePolicy at all.
	p, err = PreviewVPA(VPACreateRequest{Namespace: "a", Name: "b", TargetKind: "DaemonSet", TargetName: "c"})
	if err != nil || strings.Contains(p.YAML, "resourcePolicy") || !strings.Contains(p.YAML, `updateMode: "Off"`) {
		t.Fatalf("defaults: %v\n%s", err, p.YAML)
	}
	for _, m := range []string{"Initial", "Recreate"} {
		r := baseVPA()
		r.UpdateMode = m
		if p, _ := PreviewVPA(r); len(p.Warnings) != 1 {
			t.Errorf("%s: %v", m, p.Warnings)
		}
	}
}

func isBadRequest(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Code == http.StatusBadRequest
}

func TestBuildVPARejects(t *testing.T) {
	cases := map[string]func(*VPACreateRequest){
		"missing name":          func(r *VPACreateRequest) { r.Name = "" },
		"bad name":              func(r *VPACreateRequest) { r.Name = "Bad_Name" },
		"bad namespace":         func(r *VPACreateRequest) { r.Namespace = "a.b" },
		"bad target name":       func(r *VPACreateRequest) { r.TargetName = "-x" },
		"bad target kind":       func(r *VPACreateRequest) { r.TargetKind = "Pod" },
		"bad update mode":       func(r *VPACreateRequest) { r.UpdateMode = "Always" },
		"bad resource":          func(r *VPACreateRequest) { r.ControlledResources = []string{"gpu"} },
		"duplicate resource":    func(r *VPACreateRequest) { r.ControlledResources = []string{"cpu", "cpu"} },
		"bad quantity":          func(r *VPACreateRequest) { r.Containers[0].MinCPU = "lots" },
		"negative quantity":     func(r *VPACreateRequest) { r.Containers[0].MinMemory = "-1Mi" },
		"zero quantity":         func(r *VPACreateRequest) { r.Containers[0].MaxCPU = "0" },
		"cpu min > max":         func(r *VPACreateRequest) { r.Containers[0].MinCPU = "3" },
		"memory min > max":      func(r *VPACreateRequest) { r.Containers[0].MinMemory = "3Gi" },
		"cpu over 256 cores":    func(r *VPACreateRequest) { r.Containers[0].MaxCPU = "257" },
		"memory over 4Ti":       func(r *VPACreateRequest) { r.Containers[0].MaxMemory = "5Ti" },
		"bad container name":    func(r *VPACreateRequest) { r.Containers[0].Name = "App!" },
		"missing container":     func(r *VPACreateRequest) { r.Containers[0].Name = "" },
		"duplicate container":   func(r *VPACreateRequest) { r.Containers = append(r.Containers, r.Containers[0]) },
		"cpu bound not managed": func(r *VPACreateRequest) { r.ControlledResources = []string{"memory"} },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			r := baseVPA()
			r.Containers = append([]VPAContainerPolicyRequest{}, r.Containers...)
			mut(&r)
			if _, err := PreviewVPA(r); !isBadRequest(err) {
				t.Fatalf("want 400, got %v", err)
			}
		})
	}
	// Limits themselves are accepted.
	r := baseVPA()
	r.Containers[0].MaxCPU, r.Containers[0].MaxMemory = "256", "4Ti"
	if _, err := PreviewVPA(r); err != nil {
		t.Fatalf("limits: %v", err)
	}
}

func TestPreviewScaledObjectGolden(t *testing.T) {
	p, err := PreviewScaledObject(baseSO())
	if err != nil {
		t.Fatal(err)
	}
	if p.YAML != goldenScaledObject {
		t.Fatalf("yaml:\n%s\nwant:\n%s", p.YAML, goldenScaledObject)
	}
	if len(p.Objects) != 1 || p.Objects[0].APIVersion != "keda.sh/v1alpha1" || p.Objects[0].Kind != "ScaledObject" || p.Warnings == nil {
		t.Fatalf("preview: %+v", p)
	}
}

// One golden trigger block per type.
func TestScaledObjectTriggerGoldens(t *testing.T) {
	cases := []struct {
		trig ScaledObjectTriggerRequest
		want string
	}{
		{ScaledObjectTriggerRequest{Type: "cpu", Settings: map[string]string{"type": "Utilization", "value": "60"}},
			"  - metadata:\n      value: \"60\"\n    metricType: Utilization\n    type: cpu\n"},
		{ScaledObjectTriggerRequest{Type: "memory", Settings: map[string]string{"type": "AverageValue", "value": "512Mi"}},
			"  - metadata:\n      value: 512Mi\n    metricType: AverageValue\n    type: memory\n"},
		{ScaledObjectTriggerRequest{Type: "cron", Settings: map[string]string{"timezone": "UTC", "start": "*/30 * * * *", "end": "15,45 * * * *", "desiredReplicas": "2"}},
			"  - metadata:\n      desiredReplicas: \"2\"\n      end: 15,45 * * * *\n      start: '*/30 * * * *'\n      timezone: UTC\n    type: cron\n"},
		{ScaledObjectTriggerRequest{Type: "prometheus", Name: "rps", Settings: map[string]string{"serverAddress": "https://prom.example.com", "query": "up", "threshold": "1.5", "activationThreshold": "0", "namespace": "monitoring"}},
			"  - metadata:\n      activationThreshold: \"0\"\n      namespace: monitoring\n      query: up\n      serverAddress: https://prom.example.com\n      threshold: \"1.5\"\n    name: rps\n    type: prometheus\n"},
		{ScaledObjectTriggerRequest{Type: "kafka", Settings: map[string]string{"bootstrapServers": "kafka-0.kafka:9092,kafka-1.kafka:9092", "consumerGroup": "orders", "topic": "orders", "lagThreshold": "50", "activationLagThreshold": "5"}, AuthenticationRef: "kafka-auth"},
			"  - authenticationRef:\n      name: kafka-auth\n    metadata:\n      activationLagThreshold: \"5\"\n      bootstrapServers: kafka-0.kafka:9092,kafka-1.kafka:9092\n      consumerGroup: orders\n      lagThreshold: \"50\"\n      topic: orders\n    type: kafka\n"},
		{ScaledObjectTriggerRequest{Type: "redis", Settings: map[string]string{"address": "redis.data:6379", "listName": "jobs", "listLength": "10", "activationListLength": "1"}},
			"  - metadata:\n      activationListLength: \"1\"\n      address: redis.data:6379\n      listLength: \"10\"\n      listName: jobs\n    type: redis\n"},
		{ScaledObjectTriggerRequest{Type: "rabbitmq", Settings: map[string]string{"queueName": "tasks", "mode": "QueueLength", "value": "20", "protocol": "amqp"}, AuthenticationRef: "rabbitmq-auth"},
			"  - authenticationRef:\n      name: rabbitmq-auth\n    metadata:\n      mode: QueueLength\n      protocol: amqp\n      queueName: tasks\n      value: \"20\"\n    type: rabbitmq\n"},
		{ScaledObjectTriggerRequest{Type: "aws-sqs-queue", Settings: map[string]string{"queueURL": "https://sqs.eu-central-1.amazonaws.com/123456789012/jobs", "queueLength": "5", "awsRegion": "eu-central-1"}},
			"  - metadata:\n      awsRegion: eu-central-1\n      queueLength: \"5\"\n      queueURL: https://sqs.eu-central-1.amazonaws.com/123456789012/jobs\n    type: aws-sqs-queue\n"},
	}
	for _, c := range cases {
		t.Run(c.trig.Type, func(t *testing.T) {
			r := baseSO()
			r.MinReplicas = i32p(1)
			r.Triggers = []ScaledObjectTriggerRequest{c.trig}
			p, err := PreviewScaledObject(r)
			if err != nil {
				t.Fatal(err)
			}
			_, block, ok := strings.Cut(p.YAML, "  triggers:\n")
			if !ok || block != c.want {
				t.Fatalf("triggers block:\n%s\nwant:\n%s", block, c.want)
			}
		})
	}
}

func TestBuildScaledObjectRejects(t *testing.T) {
	trig := func(typ string, settings map[string]string, auth string) func(*ScaledObjectCreateRequest) {
		return func(r *ScaledObjectCreateRequest) {
			r.Triggers = []ScaledObjectTriggerRequest{{Type: typ, Settings: settings, AuthenticationRef: auth}}
		}
	}
	prom := func(extra map[string]string) map[string]string {
		m := map[string]string{"serverAddress": "http://prometheus.monitoring:9090", "query": "up", "threshold": "1"}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	cases := []struct {
		name   string
		mut    func(*ScaledObjectCreateRequest)
		expect string // substring of the error
	}{
		{"unknown trigger type", trig("mysql", map[string]string{"query": "x"}, ""), "allowed types are aws-sqs-queue, cpu, cron, kafka, memory, prometheus, rabbitmq, redis"},
		{"missing trigger type", trig("", nil, ""), "type is required"},
		{"no triggers", func(r *ScaledObjectCreateRequest) { r.Triggers = nil }, "at least one trigger"},
		{"unknown setting key", trig("prometheus", prom(map[string]string{"metricName": "x"}), ""), "allowed settings are activationThreshold, namespace, query, serverAddress, threshold"},
		{"credential key password", trig("prometheus", prom(map[string]string{"password": "hunter2"}), ""), "looks like a credential"},
		{"credential key bearerToken", trig("prometheus", prom(map[string]string{"bearerToken": "abc"}), ""), "looks like a credential"},
		{"credential key apiKey", trig("prometheus", prom(map[string]string{"apiKey": "abc"}), ""), "looks like a credential"},
		{"credential key sasl secret", trig("kafka", map[string]string{"bootstrapServers": "k:9092", "consumerGroup": "g", "topic": "t", "lagThreshold": "1", "saslSecret": "s"}, ""), "looks like a credential"},
		{"rabbitmq host in settings", trig("rabbitmq", map[string]string{"queueName": "q", "mode": "QueueLength", "value": "1", "host": "amqp://u:p@rabbit:5672"}, "rabbitmq-auth"), "supplied only through authenticationRef"},
		{"rabbitmq without auth", trig("rabbitmq", map[string]string{"queueName": "q", "mode": "QueueLength", "value": "1"}, ""), "authenticationRef is required"},
		{"userinfo URL", trig("prometheus", prom(map[string]string{"serverAddress": "http://admin:pw@prometheus:9090"}), ""), "must not contain credentials"},
		{"userinfo user only", trig("prometheus", prom(map[string]string{"serverAddress": "https://admin@prometheus:9090"}), ""), "must not contain credentials"},
		{"userinfo in query value", trig("prometheus", prom(map[string]string{"query": "x https://u:p@h/y"}), ""), "must not contain credentials"},
		{"non-http server", trig("prometheus", prom(map[string]string{"serverAddress": "ftp://prometheus"}), ""), "http:// or https://"},
		{"credentials in broker", trig("kafka", map[string]string{"bootstrapServers": "user:pw@k:9092", "consumerGroup": "g", "topic": "t", "lagThreshold": "1"}, ""), "must not contain credentials"},
		{"redis credentials", trig("redis", map[string]string{"address": "u:p@redis:6379", "listName": "l", "listLength": "1"}, ""), "must not contain credentials"},
		{"missing required", trig("prometheus", map[string]string{"serverAddress": "http://p:9090", "query": "up"}, ""), "threshold is required"},
		{"empty value", trig("prometheus", prom(map[string]string{"query": ""}), ""), "must not be empty"},
		{"long value", trig("prometheus", prom(map[string]string{"query": strings.Repeat("x", 513)}), ""), "at most 512"},
		{"control char", trig("prometheus", prom(map[string]string{"query": "up\nfoo"}), ""), "control characters"},
		{"bad threshold", trig("prometheus", prom(map[string]string{"threshold": "NaN"}), ""), "positive number"},
		{"bad cron", trig("cron", map[string]string{"timezone": "UTC", "start": "0 8 * *", "end": "0 18 * * *", "desiredReplicas": "1"}, ""), "5-field cron"},
		{"bad timezone", trig("cron", map[string]string{"timezone": "../etc/passwd", "start": "0 8 * * *", "end": "0 18 * * *", "desiredReplicas": "1"}, ""), "IANA time zone"},
		{"bad desired", trig("cron", map[string]string{"timezone": "UTC", "start": "0 8 * * *", "end": "0 18 * * *", "desiredReplicas": "0"}, ""), "whole number between 1"},
		{"cron with auth", trig("cron", map[string]string{"timezone": "UTC", "start": "0 8 * * *", "end": "0 18 * * *", "desiredReplicas": "1"}, "x"), "take no authenticationRef"},
		{"bad cpu type", func(r *ScaledObjectCreateRequest) {
			r.MinReplicas = i32p(1)
			trig("cpu", map[string]string{"type": "Value", "value": "1"}, "")(r)
		}, "Utilization, AverageValue"},
		{"cpu utilization not a percent", func(r *ScaledObjectCreateRequest) {
			r.MinReplicas = i32p(1)
			trig("cpu", map[string]string{"type": "Utilization", "value": "500m"}, "")(r)
		}, "percentage"},
		{"bad sqs region", trig("aws-sqs-queue", map[string]string{"queueURL": "https://sqs.x/1/q", "queueLength": "1", "awsRegion": "mars"}, ""), "AWS region"},
		{"sqs plain http", trig("aws-sqs-queue", map[string]string{"queueURL": "http://sqs.x/1/q", "queueLength": "1", "awsRegion": "eu-west-1"}, ""), "https://"},
		{"bad auth ref", func(r *ScaledObjectCreateRequest) { r.Triggers[1].AuthenticationRef = "Prom_Auth" }, "DNS-1123"},
		{"bad trigger name", func(r *ScaledObjectCreateRequest) { r.Triggers[0].Name = "Business Hours" }, "DNS-1123"},
		{"duplicate trigger name", func(r *ScaledObjectCreateRequest) { r.Triggers[1].Name = "business-hours" }, "used twice"},
		{"min > max", func(r *ScaledObjectCreateRequest) { r.MinReplicas = i32p(21) }, "between 0 and maxReplicas"},
		{"negative min", func(r *ScaledObjectCreateRequest) { r.MinReplicas = i32p(-1) }, "between 0 and maxReplicas"},
		{"max zero", func(r *ScaledObjectCreateRequest) { r.MaxReplicas = 0 }, "between 1 and 1000"},
		{"max over 1000", func(r *ScaledObjectCreateRequest) { r.MaxReplicas = 1001 }, "between 1 and 1000"},
		{"min 0 with cpu/memory only", func(r *ScaledObjectCreateRequest) {
			r.Triggers = []ScaledObjectTriggerRequest{
				{Type: "cpu", Settings: map[string]string{"type": "Utilization", "value": "60"}},
				{Type: "memory", Settings: map[string]string{"type": "Utilization", "value": "70"}},
			}
		}, "cannot wake a workload from zero"},
		{"bad polling", func(r *ScaledObjectCreateRequest) { r.PollingInterval = 4000 }, "pollingInterval"},
		{"bad cooldown", func(r *ScaledObjectCreateRequest) { r.CooldownPeriod = -1 }, "cooldownPeriod"},
		{"daemonset target", func(r *ScaledObjectCreateRequest) { r.TargetKind = "DaemonSet" }, "Deployment or StatefulSet"},
		{"bad name", func(r *ScaledObjectCreateRequest) { r.Name = "Scaler!" }, "DNS-1123"},
		{"bad namespace", func(r *ScaledObjectCreateRequest) { r.Namespace = "" }, "required"},
		{"too many triggers", func(r *ScaledObjectCreateRequest) {
			for i := 0; i < 10; i++ {
				r.Triggers = append(r.Triggers, ScaledObjectTriggerRequest{Type: "cron", Settings: map[string]string{"timezone": "UTC", "start": "0 8 * * *", "end": "0 18 * * *", "desiredReplicas": "1"}})
			}
		}, "at most 10"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := baseSO()
			r.Triggers = append([]ScaledObjectTriggerRequest{}, r.Triggers...)
			c.mut(&r)
			_, err := PreviewScaledObject(r)
			if !isBadRequest(err) {
				t.Fatalf("want 400, got %v", err)
			}
			if !strings.Contains(err.Error(), c.expect) {
				t.Fatalf("error %q does not contain %q", err, c.expect)
			}
			// Setting values (possibly credentials) are never echoed.
			for _, secret := range []string{"hunter2", "u:p@", "admin:pw", "user:pw", "../etc", "amqp://"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("error echoes a value: %v", err)
				}
			}
		})
	}
}

func TestScaledObjectDefaults(t *testing.T) {
	r := baseSO()
	r.MinReplicas, r.PollingInterval, r.CooldownPeriod = nil, 0, 0
	p, err := PreviewScaledObject(r)
	if err != nil || !strings.Contains(p.YAML, "minReplicaCount: 0\n") || !strings.Contains(p.YAML, "pollingInterval: 30\n") || !strings.Contains(p.YAML, "cooldownPeriod: 300\n") {
		t.Fatalf("event-trigger defaults: %v\n%s", err, p.YAML)
	}
	r.Triggers = []ScaledObjectTriggerRequest{{Type: "cpu", Settings: map[string]string{"type": "Utilization", "value": "60"}}}
	p, err = PreviewScaledObject(r)
	if err != nil || !strings.Contains(p.YAML, "minReplicaCount: 1\n") {
		t.Fatalf("cpu-only default min: %v\n%s", err, p.YAML)
	}
	// Explicit min 0 with an event trigger next to cpu is fine.
	r = baseSO()
	r.Triggers = append(r.Triggers, ScaledObjectTriggerRequest{Type: "cpu", Settings: map[string]string{"type": "Utilization", "value": "60"}})
	if _, err := PreviewScaledObject(r); err != nil {
		t.Fatal(err)
	}
}

func TestAutoscalerHPAConflictWarnings(t *testing.T) {
	hpas := []HPASummary{
		{Namespace: "commerce", Name: "checkout-hpa", TargetKind: "Deployment", TargetName: "checkout", TargetCPUPercent: 70,
			Metrics: []HPAMetric{{Type: "Resource", Name: "cpu"}}},
		{Namespace: "commerce", Name: "other", TargetKind: "Deployment", TargetName: "web", Metrics: []HPAMetric{{Type: "Resource", Name: "cpu"}}},
		{Namespace: "payments", Name: "keda-hpa-old", TargetKind: "Deployment", TargetName: "payments-api", Metrics: []HPAMetric{{Type: "External", Name: "s0"}}},
		{Namespace: "payments", Name: "payments-hpa", TargetKind: "Deployment", TargetName: "payments-api", Metrics: []HPAMetric{{Type: "Resource", Name: "memory"}}},
	}
	plan, _ := buildVPA(baseVPA())
	w := vpaHPAConflictWarnings(plan, hpas)
	if len(w) != 1 || !strings.Contains(w[0], "VPA and HPA on the same resource conflict") || !strings.Contains(w[0], "checkout-hpa") {
		t.Fatalf("vpa warnings: %v", w)
	}
	memOnly := baseVPA()
	memOnly.ControlledResources, memOnly.Containers = []string{"memory"}, nil
	plan, _ = buildVPA(memOnly)
	if w := vpaHPAConflictWarnings(plan, hpas); len(w) != 0 {
		t.Fatalf("memory VPA vs cpu HPA must not warn: %v", w)
	}
	so, _ := buildScaledObject(baseSO())
	w = kedaHPAConflictWarnings(so, hpas)
	if len(w) != 1 || !strings.Contains(w[0], "KEDA creates its own HPA; two autoscalers on one workload fight each other") || !strings.Contains(w[0], "payments-hpa") {
		t.Fatalf("keda warnings: %v", w)
	}
}

func TestDemoAutoscalerRoundTrip(t *testing.T) {
	ctx := context.Background()
	d := NewDemoProvider()

	if _, err := d.CreateVPA(ctx, baseVPA()); HTTPStatus(err) != http.StatusConflict {
		t.Fatalf("seeded commerce/checkout-vpa must conflict: %v", err)
	}
	req := baseVPA()
	req.Name = "checkout-rec"
	res, err := d.CreateVPA(ctx, req)
	if err != nil || !res.OK || len(res.Created) != 1 || res.Created[0].Name != "checkout-rec" {
		t.Fatalf("create vpa: %+v %v", res, err)
	}
	// The demo checkout HPA scales on cpu: create (not preview) warns.
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "VPA and HPA on the same resource conflict") {
		t.Fatalf("warnings: %v", res.Warnings)
	}
	list, _ := d.ListVPAs(ctx, "commerce")
	found := false
	for _, v := range list.Items {
		found = found || (v.Name == "checkout-rec" && v.UpdateMode == "Off" && v.Containers != nil)
	}
	if !found {
		t.Fatalf("created vpa not listed: %+v", list.Items)
	}
	if err := d.DeleteVPA(ctx, "commerce", "checkout-rec"); err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteVPA(ctx, "commerce", "checkout-rec"); HTTPStatus(err) != http.StatusNotFound {
		t.Fatalf("delete missing: %v", err)
	}

	res, err = d.CreateScaledObject(ctx, baseSO())
	if err != nil || !res.OK || res.Created[0].Kind != "ScaledObject" {
		t.Fatalf("create so: %+v %v", res, err)
	}
	t.Cleanup(func() { _ = d.DeleteScaledObject(ctx, "payments", "payments-api-scaler") })
	sl, _ := d.ListScaledObjects(ctx, "payments")
	var got *ScaledObjectSummary
	for i := range sl.Items {
		if sl.Items[i].Name == "payments-api-scaler" {
			got = &sl.Items[i]
		}
	}
	if got == nil || got.MinReplicas != 0 || got.MaxReplicas != 20 || len(got.Triggers) != 2 || got.Triggers[0].Name != "business-hours" {
		t.Fatalf("listed so: %+v", got)
	}
	b, _ := json.Marshal(sl)
	if strings.Contains(string(b), "prometheus.monitoring") || strings.Contains(string(b), "prom-auth") {
		t.Fatalf("list leaks trigger settings: %s", b)
	}
	bad := baseSO()
	bad.Name = "other"
	bad.Triggers[1].AuthenticationRef = "missing-auth"
	if _, err := d.CreateScaledObject(ctx, bad); !isBadRequest(err) || !strings.Contains(err.Error(), "TriggerAuthentication missing-auth not found in namespace payments") {
		t.Fatalf("missing auth: %v", err)
	}
	if err := d.DeleteScaledObject(ctx, "payments", "nope"); HTTPStatus(err) != http.StatusNotFound {
		t.Fatalf("delete missing so: %v", err)
	}
	tas, _ := d.ListTriggerAuthentications(ctx, "payments")
	if !tas.Installed || len(tas.Items) == 0 || tas.Items[0].Kind != "TriggerAuthentication" {
		t.Fatalf("trigger auths: %+v", tas)
	}
}
