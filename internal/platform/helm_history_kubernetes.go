//go:build kubernetes

package platform

import (
	"context"
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// loadHelmRevisions reads the release's own sh.helm.release.v1.* Secrets
// (selected by the owner=helm,name=<release> labels) and decodes only their
// redacted values and manifest.
func (k *KubernetesProvider) loadHelmRevisions(ctx context.Context, ns, name string) (map[int]HelmRevisionData, error) {
	if err := rejectFlagLike(ns, name); err != nil {
		return nil, err
	}
	list, err := k.client.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{LabelSelector: fmt.Sprintf("owner=helm,name=%s", name)})
	if err != nil {
		return nil, err
	}
	out := map[int]HelmRevisionData{}
	for _, s := range list.Items {
		raw, ok := s.Data["release"]
		if !ok {
			continue
		}
		d, err := decodeHelmRevision(raw)
		if err != nil {
			continue
		}
		if d.Revision == 0 {
			d.Revision, _ = strconv.Atoi(s.Labels["version"])
		}
		if d.Status == "" {
			d.Status = s.Labels["status"]
		}
		if d.Updated == "" {
			d.Updated = s.CreationTimestamp.Format("2006-01-02T15:04:05Z07:00")
		}
		out[d.Revision] = d
	}
	return out, nil
}

func (k *KubernetesProvider) ListHelmRevisions(ctx context.Context, ns, name string) ([]HelmRevision, error) {
	revs, err := k.loadHelmRevisions(ctx, ns, name)
	if err != nil {
		return nil, err
	}
	return helmRevisionList(revs), nil
}

func (k *KubernetesProvider) DiffHelmRevisions(ctx context.Context, ns, name string, from, to int) (HelmRevisionDiff, error) {
	revs, err := k.loadHelmRevisions(ctx, ns, name)
	if err != nil {
		return HelmRevisionDiff{}, err
	}
	return diffHelmRevisions(ns, name, revs, from, to)
}

func (k *KubernetesProvider) PreviewHelmRollback(ctx context.Context, ns, name string, revision int) (HelmRollbackPreview, error) {
	revs, err := k.loadHelmRevisions(ctx, ns, name)
	if err != nil {
		return HelmRollbackPreview{}, err
	}
	return previewHelmRollback(ns, name, revs, revision)
}

// RollbackHelm recomputes the preview, enforces the typed confirmation and then
// runs the same Job as every other Helm action, pinned to the reviewed revision.
func (k *KubernetesProvider) RollbackHelm(ctx context.Context, req HelmRollbackRequest) (HelmActionResult, error) {
	p, err := k.PreviewHelmRollback(ctx, req.Namespace, req.Name, req.Revision)
	if err != nil {
		return HelmActionResult{}, err
	}
	if err := checkHelmRollbackConfirm(p, req.Confirm); err != nil {
		return HelmActionResult{}, err
	}
	return k.HelmRollback(ctx, req.Namespace, req.Name, p.Target)
}
