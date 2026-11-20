//go:build kubernetes

package platform

import (
	"strings"
	"testing"
)

func kubeconfigWith(user, cluster string) string {
	return `apiVersion: v1
kind: Config
clusters:
- name: c
  cluster:
    server: https://203.0.113.10:6443
` + cluster + `
users:
- name: u
  user:
` + user + `
contexts:
- name: ctx
  context: {cluster: c, user: u}
current-context: ctx
`
}

func TestValidateRemoteKubeconfig(t *testing.T) {
	ok := kubeconfigWith("    token: abc123", "    insecure-skip-tls-verify: true")
	if err := validateRemoteKubeconfig([]byte(ok)); err != nil {
		t.Fatalf("an inline-token kubeconfig must be accepted: %v", err)
	}
	bad := map[string]string{
		"exec plugin":    kubeconfigWith("    exec:\n      apiVersion: client.authentication.k8s.io/v1\n      command: /bin/sh\n      args: [-c, id]", ""),
		"token file":     kubeconfigWith("    tokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token", ""),
		"client cert fs": kubeconfigWith("    client-certificate: /etc/passwd\n    client-key: /etc/shadow", ""),
		"CA file":        kubeconfigWith("    token: abc", "    certificate-authority: /etc/ssl/ca.pem"),
		"impersonate":    kubeconfigWith("    token: abc\n    as: system:admin", ""),
		"proxy url":      kubeconfigWith("    token: abc", "    proxy-url: http://127.0.0.1:8080"),
	}
	for name, kc := range bad {
		if err := validateRemoteKubeconfig([]byte(kc)); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
	if _, _, err := ProviderFromKubeconfigYAML(bad["token file"], "x"); err == nil || !strings.Contains(err.Error(), "tokenFile") {
		t.Errorf("ProviderFromKubeconfigYAML must reject tokenFile, got %v", err)
	}
}
