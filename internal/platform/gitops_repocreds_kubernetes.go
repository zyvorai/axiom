//go:build kubernetes

package platform

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GitOps repository credentials against the core API (see gitops_repocreds.go
// for the security rules). The ClusterRole already grants Secrets
// get/list/create/delete cluster-wide (deploy/k8s/rbac.yaml), so no RBAC change.

// ListGitOpsCredentials lists ONLY Secrets matching the Argo CD repository
// labels or Axiom's Flux credential labels; the rows carry no secret data.
func (k *KubernetesProvider) ListGitOpsCredentials(ctx context.Context, engine, ns string) ([]GitOpsCredential, error) {
	scopes, err := gitOpsCredListScopes(engine, ns)
	if err != nil {
		return nil, err
	}
	if k.client == nil {
		return nil, fmt.Errorf("kubernetes client unavailable")
	}
	out := []GitOpsCredential{}
	seen := map[string]bool{}
	for _, s := range scopes {
		for _, sel := range gitOpsCredSelectors(s.Engine) {
			list, lerr := k.client.CoreV1().Secrets(s.Namespace).List(ctx, metav1.ListOptions{LabelSelector: sel})
			if lerr != nil {
				if apierrors.IsNotFound(lerr) {
					continue
				}
				return nil, fmt.Errorf("list %s credentials in %s: %w", s.Engine, s.Namespace, lerr)
			}
			for i := range list.Items {
				sec := &list.Items[i]
				key := s.Engine + "/" + sec.Namespace + "/" + sec.Name
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, gitOpsCredentialSummary(s.Engine, sec.Namespace, sec.Name, sec.Labels, sec.Annotations, sec.Data, age(sec.CreationTimestamp.Time)))
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Engine != out[j].Engine {
			return out[i].Engine < out[j].Engine
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// CreateGitOpsCredential creates the Secret. It never reads or overwrites an
// existing one: a name clash is a 409 from Create itself.
func (k *KubernetesProvider) CreateGitOpsCredential(ctx context.Context, req GitOpsCredentialRequest) (GitOpsCredentialResult, error) {
	s, err := buildGitOpsCredential(req)
	if err != nil {
		return GitOpsCredentialResult{}, err
	}
	defer s.wipe()
	if k.client == nil {
		return GitOpsCredentialResult{}, fmt.Errorf("kubernetes client unavailable")
	}
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: s.Name, Namespace: s.Namespace, Labels: s.Labels, Annotations: s.Annotations},
		Type:       corev1.SecretTypeOpaque,
		Data:       s.Data,
	}
	if _, cerr := k.client.CoreV1().Secrets(s.Namespace).Create(ctx, sec, metav1.CreateOptions{FieldManager: gitOpsFieldManager}); cerr != nil {
		switch {
		case apierrors.IsAlreadyExists(cerr):
			return GitOpsCredentialResult{}, NewStatusError(http.StatusConflict, fmt.Errorf("secret %s/%s already exists; it was not changed (to rotate a credential, delete it and create it again)", s.Namespace, s.Name))
		case apierrors.IsNotFound(cerr):
			return GitOpsCredentialResult{}, notFoundf("namespace %s not found", s.Namespace)
		case apierrors.IsForbidden(cerr):
			return GitOpsCredentialResult{}, NewStatusError(http.StatusForbidden, fmt.Errorf("Axiom's service account may not create Secrets in %s", s.Namespace))
		}
		// Status reason only: never the API server's message, in case a
		// future admission webhook echoes request content.
		return GitOpsCredentialResult{}, fmt.Errorf("create secret %s/%s failed: %s", s.Namespace, s.Name, apierrors.ReasonForError(cerr))
	}
	return GitOpsCredentialResult{OK: true, Name: s.Name, Namespace: s.Namespace, Engine: s.Engine}, nil
}

// DeleteGitOpsCredential deletes a Secret only if Axiom labelled it (see
// checkGitOpsCredentialDeletable), guarded by UID against a swap in between.
func (k *KubernetesProvider) DeleteGitOpsCredential(ctx context.Context, engine, ns, name string) error {
	if err := checkGitOpsCredTarget(engine, ns, name); err != nil {
		return err
	}
	if k.client == nil {
		return fmt.Errorf("kubernetes client unavailable")
	}
	sec, err := k.client.CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return notFoundf("secret %s/%s not found", ns, name)
		}
		return fmt.Errorf("get secret %s/%s: %s", ns, name, apierrors.ReasonForError(err))
	}
	labels, uid := sec.Labels, sec.UID
	for _, b := range sec.Data {
		for i := range b {
			b[i] = 0
		}
	}
	if err := checkGitOpsCredentialDeletable(engine, labels); err != nil {
		return err
	}
	opts := metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}
	if err := k.client.CoreV1().Secrets(ns).Delete(ctx, name, opts); err != nil {
		if apierrors.IsNotFound(err) {
			return notFoundf("secret %s/%s not found", ns, name)
		}
		return fmt.Errorf("delete secret %s/%s: %s", ns, name, apierrors.ReasonForError(err))
	}
	return nil
}
