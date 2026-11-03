//go:build kubernetes

package platform

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// workloadPodSelector resolves a workload's pod selector from its spec.
func (k *KubernetesProvider) workloadPodSelector(ctx context.Context, ns, kind, name string) (labels.Selector, error) {
	var sel *metav1.LabelSelector
	switch strings.ToLower(kind) {
	case "deployment":
		d, err := k.client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		sel = d.Spec.Selector
	case "statefulset":
		s, err := k.client.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		sel = s.Spec.Selector
	case "daemonset":
		d, err := k.client.AppsV1().DaemonSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		sel = d.Spec.Selector
	case "replicaset":
		r, err := k.client.AppsV1().ReplicaSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		sel = r.Spec.Selector
	case "job":
		j, err := k.client.BatchV1().Jobs(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		sel = j.Spec.Selector
		if sel == nil {
			sel = &metav1.LabelSelector{MatchLabels: map[string]string{"job-name": name}}
		}
	default:
		return nil, NewStatusError(http.StatusBadRequest, fmt.Errorf("log aggregation is not supported for kind %q", kind))
	}
	if sel == nil {
		return nil, NewStatusError(http.StatusUnprocessableEntity, fmt.Errorf("%s %s/%s has no pod selector", kind, ns, name))
	}
	s, err := metav1.LabelSelectorAsSelector(sel)
	if err != nil {
		return nil, err
	}
	if s.Empty() {
		return nil, NewStatusError(http.StatusUnprocessableEntity, fmt.Errorf("%s %s/%s has an empty pod selector", kind, ns, name))
	}
	return s, nil
}

// WorkloadLogs merges the logs of every pod/container of a workload into one
// stream of "[pod/container] <timestamp> <line>" lines (see mergeLogStreams).
// Containers that cannot produce logs yet (still waiting to start, or no
// previous instance when Previous is set) are skipped; the call fails only
// when no container yields a stream.
func (k *KubernetesProvider) WorkloadLogs(ctx context.Context, ns, kind, name string, opts WorkloadLogOptions) (io.ReadCloser, error) {
	sel, err := k.workloadPodSelector(ctx, ns, kind, name)
	if err != nil {
		return nil, err
	}
	pods, err := k.client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: sel.String()})
	if err != nil {
		return nil, err
	}
	sort.Slice(pods.Items, func(i, j int) bool { return pods.Items[i].Name < pods.Items[j].Name })
	var (
		streams  []labeledLogStream
		firstErr error
	)
	closeAll := func() {
		for _, s := range streams {
			_ = s.rc.Close()
		}
	}
	for _, p := range pods.Items {
		for _, c := range p.Spec.Containers {
			if opts.Container != "" && c.Name != opts.Container {
				continue
			}
			if len(streams) >= MaxWorkloadLogStreams {
				break
			}
			rc, err := k.PodLogs(ctx, ns, p.Name, c.Name, opts.Follow, opts.tail(), opts.Previous)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			streams = append(streams, labeledLogStream{label: p.Name + "/" + c.Name, rc: rc})
		}
	}
	if len(streams) == 0 {
		closeAll()
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, notFoundf("%s %s/%s has no pods to read logs from", kind, ns, name)
	}
	return mergeLogStreams(streams, opts.Follow), nil
}
