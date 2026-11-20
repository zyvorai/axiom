package platform

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"sync"
)

// Demo GitOps repository credentials. Only the listing metadata and labels
// are kept: the demo never stores secret values at all, so there is nothing
// to leak. Label semantics match the Kubernetes provider (delete refuses
// Secrets Axiom did not label).

type demoGitOpsCred struct {
	cred   GitOpsCredential
	labels map[string]string
}

var (
	demoGitOpsCredMu sync.Mutex
	demoGitOpsCreds  = map[string]demoGitOpsCred{
		// Created by hand in Argo CD: listed, but not Axiom's to delete.
		"argocd/argocd/repo-platform": {
			cred: GitOpsCredential{Name: "repo-platform", Engine: GitOpsCredEngineArgoCD, Kind: "git",
				URL: "https://github.com/zyvorai/platform.git", AuthType: "basic", Namespace: "argocd", Age: "30d"},
			labels: map[string]string{ArgoSecretTypeLabel: "repository"},
		},
	}
)

func demoGitOpsCredKey(engine, ns, name string) string { return engine + "/" + ns + "/" + name }

func (d *DemoProvider) ListGitOpsCredentials(ctx context.Context, engine, ns string) ([]GitOpsCredential, error) {
	scopes, err := gitOpsCredListScopes(engine, ns)
	if err != nil {
		return nil, err
	}
	demoGitOpsCredMu.Lock()
	defer demoGitOpsCredMu.Unlock()
	out := []GitOpsCredential{}
	for _, c := range demoGitOpsCreds {
		for _, s := range scopes {
			if c.cred.Engine == s.Engine && c.cred.Namespace == s.Namespace && demoCredSelected(s.Engine, c.labels) {
				out = append(out, c.cred)
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

// demoCredSelected mirrors the label selectors the Kubernetes listing uses.
func demoCredSelected(engine string, labels map[string]string) bool {
	if engine == GitOpsCredEngineArgoCD {
		t := labels[ArgoSecretTypeLabel]
		return t == "repository" || t == "repo-creds"
	}
	return labels[GitOpsManagedByLabel] == GitOpsManagedByValue && labels[GitOpsCredentialPurposeLabel] == GitOpsCredentialPurposeValue
}

func (d *DemoProvider) CreateGitOpsCredential(ctx context.Context, req GitOpsCredentialRequest) (GitOpsCredentialResult, error) {
	s, err := buildGitOpsCredential(req)
	if err != nil {
		return GitOpsCredentialResult{}, err
	}
	defer s.wipe()
	demoGitOpsCredMu.Lock()
	defer demoGitOpsCredMu.Unlock()
	key := demoGitOpsCredKey(s.Engine, s.Namespace, s.Name)
	// Secret names are per namespace, whatever the engine.
	for _, c := range demoGitOpsCreds {
		if c.cred.Namespace == s.Namespace && c.cred.Name == s.Name {
			return GitOpsCredentialResult{}, NewStatusError(http.StatusConflict, fmt.Errorf("secret %s/%s already exists", s.Namespace, s.Name))
		}
	}
	demoGitOpsCreds[key] = demoGitOpsCred{
		cred:   gitOpsCredentialSummary(s.Engine, s.Namespace, s.Name, s.Labels, s.Annotations, nil, "0s"),
		labels: s.Labels,
	}
	return GitOpsCredentialResult{OK: true, Name: s.Name, Namespace: s.Namespace, Engine: s.Engine}, nil
}

func (d *DemoProvider) DeleteGitOpsCredential(ctx context.Context, engine, ns, name string) error {
	if err := checkGitOpsCredTarget(engine, ns, name); err != nil {
		return err
	}
	demoGitOpsCredMu.Lock()
	defer demoGitOpsCredMu.Unlock()
	key := demoGitOpsCredKey(engine, ns, name)
	c, ok := demoGitOpsCreds[key]
	if !ok {
		return notFoundf("secret %s/%s not found", ns, name)
	}
	if err := checkGitOpsCredentialDeletable(engine, c.labels); err != nil {
		return err
	}
	delete(demoGitOpsCreds, key)
	return nil
}
