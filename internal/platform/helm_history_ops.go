package platform

import (
	"fmt"
	"sort"
)

// helmRevisionList returns the history, newest first.
func helmRevisionList(revs map[int]HelmRevisionData) []HelmRevision {
	out := make([]HelmRevision, 0, len(revs))
	for _, r := range revs {
		out = append(out, r.HelmRevision)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Revision > out[j].Revision })
	return out
}

func helmRevisionNotFound(ns, name string, rev int) error {
	return NewStatusError(404, fmt.Errorf("release %s/%s has no revision %d", ns, name, rev))
}

// diffHelmRevisions diffs two revisions from a decoded history.
func diffHelmRevisions(ns, name string, revs map[int]HelmRevisionData, from, to int) (HelmRevisionDiff, error) {
	a, ok := revs[from]
	if !ok {
		return HelmRevisionDiff{}, helmRevisionNotFound(ns, name, from)
	}
	b, ok := revs[to]
	if !ok {
		return HelmRevisionDiff{}, helmRevisionNotFound(ns, name, to)
	}
	return buildHelmDiff(ns, name, a, b), nil
}

// previewHelmRollback resolves the target (0 = the previous revision) and
// reviews the rollback from the live revision.
func previewHelmRollback(ns, name string, revs map[int]HelmRevisionData, target int) (HelmRollbackPreview, error) {
	list := helmRevisionList(revs)
	if len(list) == 0 {
		return HelmRollbackPreview{}, NewStatusError(404, fmt.Errorf("release %s/%s not found", ns, name))
	}
	live := liveHelmRevision(list)
	if target <= 0 {
		target = previousHelmRevision(list, live)
		if target == 0 {
			return HelmRollbackPreview{}, NewStatusError(409, fmt.Errorf("release %s/%s has no previous revision", ns, name))
		}
	}
	t, ok := revs[target]
	if !ok {
		return HelmRollbackPreview{}, helmRevisionNotFound(ns, name, target)
	}
	return buildHelmRollbackPreview(ns, name, revs[live], t), nil
}
