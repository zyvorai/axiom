//go:build kubernetes

package platform

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const defaultHelmImage = "alpine/helm:3.14.4"

// helmJobConfig returns where Helm Jobs run and as whom. They previously
// hard-coded the "axiom-system" namespace, the "zyvor-axiom" ServiceAccount and
// a public image, so a release installed anywhere else failed and air-gapped
// clusters could not pull alpine/helm. AXIOM_HELM_IMAGE and
// AXIOM_SERVICE_ACCOUNT override the image and ServiceAccount.
func helmJobConfig() (ns, serviceAccount, image string) {
	ns = axiomNamespace()
	serviceAccount = strings.TrimSpace(getenv("AXIOM_SERVICE_ACCOUNT"))
	if serviceAccount == "" {
		serviceAccount = "zyvor-axiom"
	}
	image = strings.TrimSpace(getenv("AXIOM_HELM_IMAGE"))
	if image == "" {
		image = defaultHelmImage
	}
	return ns, serviceAccount, image
}

// helmJobName builds a unique, DNS-1123-safe Job name of at most 63 characters:
// axiom-helm-<action>-<release>-<suffix>. The release part is truncated to fit
// (a 36+ character release name used to push the name past 63 and the Job
// create failed), and the suffix is sub-second so two requests in the same
// second don't collide.
func helmJobName(action, release string) string {
	suffix := fmt.Sprintf("%d", time.Now().UnixNano()/int64(time.Millisecond)%1000000)
	prefix := "axiom-helm-" + sanitizeDNS(action) + "-"
	room := 63 - len(prefix) - 1 - len(suffix)
	rel := sanitizeDNS(release)
	if len(rel) > room {
		rel = strings.Trim(rel[:room], "-")
	}
	if rel == "" {
		return prefix + suffix
	}
	return prefix + rel + "-" + suffix
}

// rejectFlagLike refuses values that would be parsed by helm as flags. Every
// argument is a literal argv element (no shell), so this is not about command
// injection, but a release/chart/repo/version beginning with "-" could still
// smuggle an option such as --post-renderer or --kubeconfig.
func rejectFlagLike(fields ...string) error {
	for _, f := range fields {
		if strings.HasPrefix(strings.TrimSpace(f), "-") {
			return fmt.Errorf("invalid value %q: must not start with '-'", f)
		}
	}
	return nil
}

// createHelmValuesSecret stores chart values in a Secret (they routinely hold
// passwords — a ConfigMap is plaintext-readable by anyone with configmap read)
// and returns the volume and mount that expose it to the Job.
func (k *KubernetesProvider) createHelmValuesSecret(ctx context.Context, ns, jobName, values string) (corev1.Volume, corev1.VolumeMount, string, error) {
	name := jobName + "-values"
	_, err := k.client.CoreV1().Secrets(ns).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: ns,
			Labels: map[string]string{"app.kubernetes.io/managed-by": "zyvor-axiom"},
		},
		Type:       corev1.SecretTypeOpaque,
		StringData: map[string]string{"values.yaml": values},
		Data:       map[string][]byte{"values.yaml": []byte(values)},
	}, metav1.CreateOptions{})
	if err != nil {
		return corev1.Volume{}, corev1.VolumeMount{}, "", fmt.Errorf("create values secret: %w", err)
	}
	return corev1.Volume{
		Name:         "values",
		VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: name}},
	}, corev1.VolumeMount{
		Name: "values", MountPath: "/values", ReadOnly: true,
	}, name, nil
}

// helmRegistryConfigPath is where the registry Secret is mounted; helm (3.8+)
// reads registry credentials from $HELM_REGISTRY_CONFIG.
const helmRegistryConfigPath = "/registry/config.json"

// helmOCIJobAccess applies a resolved HelmOCIAccess to an OCI helm Job:
// --plain-http for a plain-http repository, and for a repository with
// credentials a short-lived Secret holding a Docker-style config.json,
// mounted read-only, with HELM_REGISTRY_CONFIG pointing at it. The password
// is only ever in that Secret: never in the Job's args or env. The returned
// Secret name ("" for none) must be adopted by the Job once it exists (or
// deleted if the Job cannot be created).
func (k *KubernetesProvider) helmOCIJobAccess(ctx context.Context, ns, jobName string, req InstallHelmRequest) (args []string, vols []corev1.Volume, mounts []corev1.VolumeMount, env []corev1.EnvVar, secretName string, err error) {
	a := req.OCIAccess
	if req.OCIRef == "" || a == nil {
		return nil, nil, nil, nil, "", nil
	}
	loc, err := parseHelmOCIURL(req.OCIRef)
	if err != nil || loc.host != a.registry {
		return nil, nil, nil, nil, "", NewStatusError(http.StatusBadRequest, errors.New("the resolved registry access does not match ociRef"))
	}
	if a.plainHTTP {
		args = append(args, "--plain-http")
	}
	if !a.authenticated() {
		return args, nil, nil, nil, "", nil
	}
	cfg, err := a.dockerConfig()
	if err != nil {
		return nil, nil, nil, nil, "", err
	}
	defer func() {
		for i := range cfg {
			cfg[i] = 0
		}
	}()
	name := jobName + "-registry"
	if _, err := k.client.CoreV1().Secrets(ns).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: ns,
			Labels: map[string]string{"app.kubernetes.io/managed-by": "zyvor-axiom"},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{"config.json": cfg},
	}, metav1.CreateOptions{}); err != nil {
		// The API error names the Secret, never its data.
		return nil, nil, nil, nil, "", fmt.Errorf("create registry credentials secret: %w", err)
	}
	vols = []corev1.Volume{{
		Name:         "registry",
		VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: name}},
	}}
	mounts = []corev1.VolumeMount{{Name: "registry", MountPath: "/registry", ReadOnly: true}}
	env = []corev1.EnvVar{{Name: "HELM_REGISTRY_CONFIG", Value: helmRegistryConfigPath}}
	return args, vols, mounts, env, name, nil
}

// deleteHelmJobSecret removes a Secret made for a Job that was never created.
func (k *KubernetesProvider) deleteHelmJobSecret(ctx context.Context, ns, name string) {
	if name == "" {
		return
	}
	if err := k.client.CoreV1().Secrets(ns).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		log.Printf("helm: could not delete secret %s/%s: %v", ns, name, err)
	}
}

// adoptHelmValuesSecret makes the Job the owner of its values Secret, so the
// Secret is garbage-collected when the Job's TTL expires instead of leaving
// chart passwords behind forever. Best effort: if the Job has no UID yet
// (fake clientset) or the update fails, it is logged, not fatal.
func (k *KubernetesProvider) adoptHelmValuesSecret(ctx context.Context, ns, secretName string, job *batchv1.Job) {
	if secretName == "" || job == nil || job.UID == "" {
		return
	}
	sec, err := k.client.CoreV1().Secrets(ns).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		log.Printf("helm: could not adopt values secret %s/%s: %v", ns, secretName, err)
		return
	}
	sec.OwnerReferences = []metav1.OwnerReference{{APIVersion: "batch/v1", Kind: "Job", Name: job.Name, UID: job.UID}}
	if _, err := k.client.CoreV1().Secrets(ns).Update(ctx, sec, metav1.UpdateOptions{}); err != nil {
		log.Printf("helm: could not adopt values secret %s/%s: %v", ns, secretName, err)
	}
}
