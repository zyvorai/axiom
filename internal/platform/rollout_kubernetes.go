//go:build kubernetes

package platform

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func rolloutSupportedKind(kind string) bool {
	switch strings.ToLower(kind) {
	case "deployment", "statefulset", "daemonset":
		return true
	default:
		return false
	}
}

func (k *KubernetesProvider) GetRolloutStatus(ctx context.Context, ns, kind, name string) (RolloutStatus, error) {
	if !rolloutSupportedKind(kind) {
		return RolloutStatus{}, fmt.Errorf("rollout not supported for %s", kind)
	}
	switch strings.ToLower(kind) {
	case "deployment":
		dep, err := k.client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return RolloutStatus{}, err
		}
		status := dep.Status.String()
		if len(dep.Status.Conditions) > 0 {
			status = string(dep.Status.Conditions[len(dep.Status.Conditions)-1].Type)
			if dep.Status.Conditions[len(dep.Status.Conditions)-1].Status == "False" {
				status = dep.Status.Conditions[len(dep.Status.Conditions)-1].Reason
			}
		}
		history, _ := k.deploymentRevisionHistory(ctx, ns, name)
		paused := dep.Spec.Paused
		return RolloutStatus{
			Status:    status,
			Paused:    paused,
			Replicas:  dep.Status.Replicas,
			Ready:     dep.Status.ReadyReplicas,
			Updated:   dep.Status.UpdatedReplicas,
			Available: dep.Status.AvailableReplicas,
			History:   history,
		}, nil
	case "statefulset":
		sts, err := k.client.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return RolloutStatus{}, err
		}
		return RolloutStatus{
			Status:    fmt.Sprintf("%d/%d ready", sts.Status.ReadyReplicas, sts.Status.Replicas),
			Replicas:  sts.Status.Replicas,
			Ready:     sts.Status.ReadyReplicas,
			Updated:   sts.Status.UpdatedReplicas,
			Available: sts.Status.ReadyReplicas,
		}, nil
	default:
		return RolloutStatus{}, fmt.Errorf("rollout not supported for %s", kind)
	}
}

func (k *KubernetesProvider) deploymentRevisionHistory(ctx context.Context, ns, name string) ([]RolloutRevision, error) {
	rsList, err := k.client.AppsV1().ReplicaSets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	type revItem struct {
		revision int
		status   string
		age      time.Time
	}
	var items []revItem
	for _, rs := range rsList.Items {
		for _, ref := range rs.OwnerReferences {
			if ref.Kind == "Deployment" && ref.Name == name {
				revStr := rs.Annotations["deployment.kubernetes.io/revision"]
				rev, _ := strconv.Atoi(revStr)
				status := "inactive"
				if rs.Status.Replicas > 0 {
					status = "active"
				}
				items = append(items, revItem{revision: rev, status: status, age: rs.CreationTimestamp.Time})
			}
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].revision > items[j].revision })
	out := make([]RolloutRevision, 0, len(items))
	for i, it := range items {
		if i >= 5 {
			break
		}
		out = append(out, RolloutRevision{Revision: it.revision, Status: it.status, Age: age(it.age)})
	}
	return out, nil
}

func (k *KubernetesProvider) RolloutAction(ctx context.Context, ns, kind, name, action string) error {
	if !rolloutSupportedKind(kind) {
		return fmt.Errorf("rollout not supported for %s", kind)
	}
	switch strings.ToLower(kind) {
	case "deployment":
		return k.deploymentRolloutAction(ctx, ns, name, action)
	case "statefulset":
		if action == "restart" {
			return k.restartStatefulSet(ctx, ns, name)
		}
		return fmt.Errorf("action %s not supported for StatefulSet", action)
	case "daemonset":
		if action == "restart" {
			return k.Restart(ctx, ns, kind, name)
		}
		return fmt.Errorf("action %s not supported for DaemonSet", action)
	default:
		return fmt.Errorf("rollout not supported for %s", kind)
	}
}

func (k *KubernetesProvider) deploymentRolloutAction(ctx context.Context, ns, name, action string) error {
	switch action {
	case "pause":
		patch := []byte(`{"spec":{"paused":true}}`)
		_, err := k.client.AppsV1().Deployments(ns).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{})
		return err
	case "resume":
		patch := []byte(`{"spec":{"paused":false}}`)
		_, err := k.client.AppsV1().Deployments(ns).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{})
		return err
	case "restart":
		return k.restartDeployment(ctx, ns, name)
	case "undo":
		return k.undoDeployment(ctx, ns, name)
	default:
		return fmt.Errorf("unknown rollout action: %s", action)
	}
}

func (k *KubernetesProvider) restartDeployment(ctx context.Context, ns, name string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`, now)
	_, err := k.client.AppsV1().Deployments(ns).Patch(ctx, name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	return err
}

func (k *KubernetesProvider) restartStatefulSet(ctx context.Context, ns, name string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`, now)
	_, err := k.client.AppsV1().StatefulSets(ns).Patch(ctx, name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	return err
}

func (k *KubernetesProvider) undoDeployment(ctx context.Context, ns, name string) error {
	if _, err := k.client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{}); err != nil {
		return err // a missing Deployment is a 404, not "no previous revision"
	}
	history, err := k.deploymentRevisionHistory(ctx, ns, name)
	if err != nil {
		return err
	}
	if len(history) < 2 {
		return NewStatusError(http.StatusConflict, fmt.Errorf("no previous revision to undo to"))
	}
	targetRev := history[1].Revision
	rsList, err := k.client.AppsV1().ReplicaSets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	var target *appsv1.ReplicaSet
	for i := range rsList.Items {
		rs := &rsList.Items[i]
		for _, ref := range rs.OwnerReferences {
			if ref.Kind == "Deployment" && ref.Name == name {
				revStr := rs.Annotations["deployment.kubernetes.io/revision"]
				rev, _ := strconv.Atoi(revStr)
				if rev == targetRev {
					target = rs
					break
				}
			}
		}
	}
	if target == nil {
		return notFoundf("revision %d not found", targetRev)
	}
	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"deployment.kubernetes.io/revision":"%d"}}}}}`, targetRev)
	_, err = k.client.AppsV1().Deployments(ns).Patch(ctx, name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{FieldManager: "axiom-rollout"})
	if err != nil {
		return err
	}
	// Roll back pod template to match target RS
	dep, err := k.client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	dep.Spec.Template = target.Spec.Template
	_, err = k.client.AppsV1().Deployments(ns).Update(ctx, dep, metav1.UpdateOptions{})
	return err
}
