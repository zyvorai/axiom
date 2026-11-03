package platform

import (
	"context"
	"strings"
)

// Deterministic demo scanner output, shaped exactly like the real CRs so the
// demo build runs the same parsing code as the Kubernetes build.

func demoVulnReport(ns, kind, name, container, repo, tag string, crit, high, med, low int, vulns ...map[string]any) map[string]any {
	list := make([]any, 0, len(vulns))
	for _, v := range vulns {
		list = append(list, v)
	}
	return map[string]any{
		"metadata": map[string]any{
			"name": strings.ToLower(kind) + "-" + name + "-" + container, "namespace": ns,
			"labels": map[string]any{
				"trivy-operator.resource.kind": kind, "trivy-operator.resource.name": name,
				"trivy-operator.resource.namespace": ns, "trivy-operator.container.name": container,
			},
		},
		"report": map[string]any{
			"artifact":        map[string]any{"repository": repo, "tag": tag},
			"registry":        map[string]any{"server": "ghcr.io"},
			"summary":         map[string]any{"criticalCount": int64(crit), "highCount": int64(high), "mediumCount": int64(med), "lowCount": int64(low), "unknownCount": int64(0)},
			"vulnerabilities": list,
		},
	}
}

func demoVuln(id, sev, pkg, installed, fixed, title string, score float64) map[string]any {
	return map[string]any{
		"vulnerabilityID": id, "severity": sev, "resource": pkg, "installedVersion": installed,
		"fixedVersion": fixed, "title": title, "score": score, "primaryLink": "https://avd.aquasec.com/nvd/" + strings.ToLower(id),
	}
}

func demoConfigReport(ns, kind, name string, crit, high, med, low int, checks ...map[string]any) map[string]any {
	list := make([]any, 0, len(checks))
	for _, c := range checks {
		list = append(list, c)
	}
	return map[string]any{
		"metadata": map[string]any{
			"name": strings.ToLower(kind) + "-" + name, "namespace": ns,
			"labels": map[string]any{
				"trivy-operator.resource.kind": kind, "trivy-operator.resource.name": name, "trivy-operator.resource.namespace": ns,
			},
		},
		"report": map[string]any{
			"summary": map[string]any{"criticalCount": int64(crit), "highCount": int64(high), "mediumCount": int64(med), "lowCount": int64(low)},
			"checks":  list,
		},
	}
}

func demoCheck(id, sev, title, category string, success bool, msgs ...string) map[string]any {
	ms := make([]any, 0, len(msgs))
	for _, m := range msgs {
		ms = append(ms, m)
	}
	return map[string]any{"checkID": id, "severity": sev, "title": title, "category": category, "success": success, "messages": ms}
}

func demoPolicyReport(ns, kind, name string, results ...map[string]any) map[string]any {
	list := make([]any, 0, len(results))
	for _, r := range results {
		list = append(list, r)
	}
	return map[string]any{
		"metadata": map[string]any{"name": "polr-ns-" + ns + "-" + strings.ToLower(kind) + "-" + name, "namespace": ns},
		"scope":    map[string]any{"kind": kind, "name": name, "namespace": ns},
		"results":  list,
	}
}

func demoPolicyResult(policy, rule, result, severity, msg string) map[string]any {
	return map[string]any{"policy": policy, "rule": rule, "result": result, "severity": severity, "message": msg, "source": "kyverno"}
}

func demoSecurityReports() secReportSet {
	vuln := []map[string]any{
		demoVulnReport("payments", "ReplicaSet", "payments-api-7d9f8c6b5", "payments-api", "zyvorai/payments", "v2.8.4", 1, 3, 2, 1,
			demoVuln("CVE-2024-45337", "CRITICAL", "golang.org/x/crypto", "v0.28.0", "v0.31.0", "Misuse of ServerConfig.PublicKeyCallback may cause authorization bypass", 9.1),
			demoVuln("CVE-2024-24790", "HIGH", "stdlib", "1.21.4", "1.21.11", "net/netip: unexpected behavior from Is methods for IPv4-mapped IPv6 addresses", 7.5),
			demoVuln("CVE-2023-45288", "HIGH", "golang.org/x/net", "v0.17.0", "v0.23.0", "http2: close connections when receiving too many headers", 7.5),
			demoVuln("CVE-2024-34156", "HIGH", "stdlib", "1.21.4", "1.22.7", "encoding/gob: stack exhaustion in Decoder.Decode", 7.5),
			demoVuln("CVE-2024-2511", "MEDIUM", "libssl3", "3.0.13-1", "3.0.14-1", "openssl: unbounded memory growth with session handling in TLSv1.3", 5.9),
			demoVuln("CVE-2024-4741", "MEDIUM", "libssl3", "3.0.13-1", "3.0.14-1", "openssl: use-after-free with SSL_free_buffers", 5.0),
			demoVuln("CVE-2023-31484", "LOW", "perl-base", "5.36.0-7", "", "perl: CPAN.pm does not verify TLS certificates", 3.7),
		),
		demoVulnReport("commerce", "ReplicaSet", "checkout-5c8b7d9f4", "checkout", "zyvorai/checkout", "v1.14.2", 0, 2, 1, 0,
			demoVuln("CVE-2024-6387", "HIGH", "openssh-client", "1:9.2p1-2", "1:9.2p1-2+deb12u3", "openssh: regreSSHion race condition in signal handler", 8.1),
			demoVuln("CVE-2024-0727", "HIGH", "libssl3", "3.0.11-1", "3.0.13-1", "openssl: denial of service via null dereference", 5.5),
			demoVuln("CVE-2023-5678", "MEDIUM", "libssl3", "3.0.11-1", "3.0.12-1", "openssl: Excessive time spent in DH check", 5.3),
		),
		demoVulnReport("data", "StatefulSet", "redis", "redis", "library/redis", "7.4", 0, 0, 1, 2,
			demoVuln("CVE-2024-31449", "MEDIUM", "redis", "7.4.0", "7.4.1", "redis: Lua library commands may lead to stack overflow", 7.0),
			demoVuln("CVE-2022-3219", "LOW", "gpgv", "2.2.40-1.1", "", "gnupg: denial of service issue (resource consumption) using compressed packets", 3.3),
			demoVuln("CVE-2011-3374", "LOW", "apt", "2.6.1", "", "It was found that apt-key in apt does not correctly validate gpg keys", 3.7),
		),
		demoVulnReport("observability", "DaemonSet", "log-shipper", "fluent-bit", "fluent/fluent-bit", "3.1", 0, 0, 0, 0),
	}
	config := []map[string]any{
		demoConfigReport("payments", "ReplicaSet", "payments-api-7d9f8c6b5", 0, 1, 1, 0,
			demoCheck("KSV017", "HIGH", "Privileged container", "Kubernetes Security Check", false, "Container 'payments-api' of Deployment 'payments-api' should set 'securityContext.privileged' to false"),
			demoCheck("KSV014", "MEDIUM", "Root file system is not read-only", "Kubernetes Security Check", false, "Container 'payments-api' of Deployment 'payments-api' should set 'securityContext.readOnlyRootFilesystem' to true"),
			demoCheck("KSV001", "MEDIUM", "Process can elevate its own privileges", "Kubernetes Security Check", true),
		),
		demoConfigReport("data", "StatefulSet", "redis", 0, 0, 1, 1,
			demoCheck("KSV106", "LOW", "Container capabilities must only include NET_BIND_SERVICE", "Kubernetes Security Check", false, "Container 'redis' of StatefulSet 'redis' should drop all capabilities"),
			demoCheck("KSV011", "LOW", "CPU not limited", "Kubernetes Security Check", false, "Container 'redis' of StatefulSet 'redis' should set 'resources.limits.cpu'"),
		),
	}
	policy := []map[string]any{
		demoPolicyReport("commerce", "Deployment", "checkout",
			demoPolicyResult("require-run-as-nonroot", "run-as-non-root", "fail", "high", "securityContext.runAsNonRoot must be true"),
			demoPolicyResult("require-requests-limits", "validate-resources", "pass", "medium", ""),
			demoPolicyResult("disallow-latest-tag", "require-image-tag", "pass", "medium", ""),
		),
		demoPolicyReport("payments", "Deployment", "payments-api",
			demoPolicyResult("require-run-as-nonroot", "run-as-non-root", "pass", "high", ""),
			demoPolicyResult("require-requests-limits", "validate-resources", "warn", "medium", "CPU limit is recommended"),
		),
	}
	return secReportSet{
		Vuln: vuln, Config: config, Policy: policy,
		Sources: []SecuritySource{
			{Name: "Trivy VulnerabilityReports", Group: "aquasecurity.github.io", Available: true, Reports: len(vuln)},
			{Name: "Trivy ConfigAuditReports", Group: "aquasecurity.github.io", Available: true, Reports: len(config)},
			{Name: "PolicyReports", Group: "wgpolicyk8s.io", Available: true, Reports: len(policy)},
		},
	}
}

func filterSecurityNamespace(set secReportSet, ns string) secReportSet {
	if ns == "" || ns == "all" {
		return set
	}
	pick := func(in []map[string]any) []map[string]any {
		var out []map[string]any
		for _, o := range in {
			if ustr(uget(o, "metadata", "namespace")) == ns {
				out = append(out, o)
			}
		}
		return out
	}
	set.Vuln, set.Config, set.Policy = pick(set.Vuln), pick(set.Config), pick(set.Policy)
	srcs := append([]SecuritySource(nil), set.Sources...)
	for i := range srcs {
		switch srcs[i].Group + "/" + srcs[i].Name {
		case "aquasecurity.github.io/Trivy VulnerabilityReports":
			srcs[i].Reports = len(set.Vuln)
		case "aquasecurity.github.io/Trivy ConfigAuditReports":
			srcs[i].Reports = len(set.Config)
		case "wgpolicyk8s.io/PolicyReports":
			srcs[i].Reports = len(set.Policy)
		}
	}
	set.Sources = srcs
	return set
}

func (d *DemoProvider) ListSecurityReports(ctx context.Context, ns string) (SecurityReportSummary, error) {
	return filterSecurityNamespace(demoSecurityReports(), ns).summary(), nil
}

func (d *DemoProvider) GetWorkloadSecurity(ctx context.Context, ns, kind, name string) (WorkloadSecurityDetail, error) {
	if err := validateWorkloadRef(ns, kind, name); err != nil {
		return WorkloadSecurityDetail{}, err
	}
	return filterSecurityNamespace(demoSecurityReports(), ns).detail(ns, kind, name), nil
}
