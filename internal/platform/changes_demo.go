package platform

import "time"

// changeSeeder lets a provider contribute extra change candidates; only the
// demo provider implements it, so demo mode shows a realistic mix of sources
// relative to "now" without needing live GitOps/Helm/node history.
type changeSeeder interface {
	seedChanges(now time.Time) []ChangeCandidate
}

func (d *DemoProvider) seedChanges(now time.Time) []ChangeCandidate {
	m := func(n int) time.Time { return now.Add(-time.Duration(n) * time.Minute) }
	return []ChangeCandidate{
		{Source: ChangeGitOps, Time: m(14), Namespace: "argocd", Object: "Application/platform-apps", Actor: "argocd-controller",
			Summary: "Application platform-apps synced revision a1b2c3d (payments-api image 1.42.0 -> 1.43.0)"},
		{Source: ChangeRollout, Time: m(13), Namespace: "payments", Object: "Deployment/payments-api", Actor: "argocd-controller",
			Summary: "Rollout revision 18 created: image payments-api:1.43.0"},
		{Source: ChangeConfig, Time: m(22), Namespace: "payments", Object: "ConfigMap/payments-api-config", Actor: "alice@example.com",
			Summary: "configmap.update payments/payments-api-config (LOG_LEVEL=debug)"},
		{Source: ChangeConfig, Time: m(35), Namespace: "payments", Object: "Secret/payments-db", Actor: "bob@example.com",
			Summary: "secret.update payments/payments-db (key rotated)"},
		{Source: ChangeHelm, Time: m(41), Namespace: "monitoring", Object: "HelmRelease/prometheus", Actor: "ci-bot",
			Summary: "Helm release prometheus upgraded to revision 8 (kube-prometheus-stack-61.3.0)"},
		{Source: ChangeHPA, Time: m(9), Namespace: "payments", Object: "Deployment/payments-api",
			Summary: "HPA payments-api scaled 6 -> 12 replicas (cpu 91%)"},
		{Source: ChangeNode, Time: m(27), Object: "Node/worker-2", Actor: "alice@example.com",
			Summary: "node.cordon worker-2 (maintenance)"},
		{Source: ChangeNode, Time: m(6), Object: "Node/worker-3",
			Summary: "NodeNotReady: Node worker-3 status is now: NodeNotReady"},
		{Source: ChangeAudit, Time: m(18), Namespace: "commerce", Object: "Deployment/checkout", Actor: "carol@example.com",
			Summary: "workload.scale commerce/Deployment/checkout (3 -> 5)"},
	}
}
