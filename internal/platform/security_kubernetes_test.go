//go:build kubernetes

package platform

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/remotecommand"
)

func TestKubernetesDeleteNamespaceProtected(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}})
	err := k.DeleteNamespace(ctx, "kube-system")
	if err == nil || HTTPStatus(err) != 403 {
		t.Fatalf("want 403, got %v", err)
	}
	if _, gerr := cs.CoreV1().Namespaces().Get(ctx, "kube-system", metav1.GetOptions{}); gerr != nil {
		t.Fatalf("protected namespace must survive: %v", gerr)
	}
}

func TestPodProxyTargetRestrictions(t *testing.T) {
	ctx := context.Background()
	mk := func(name string, hostNet bool, ports ...int32) *corev1.Pod {
		var cps []corev1.ContainerPort
		for _, p := range ports {
			cps = append(cps, corev1.ContainerPort{ContainerPort: p})
		}
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"},
			Spec:       corev1.PodSpec{HostNetwork: hostNet, Containers: []corev1.Container{{Name: "c", Ports: cps}}},
			Status:     corev1.PodStatus{PodIP: "10.0.0.5"},
		}
	}
	k, _ := newFakeKubernetesProvider(mk("web", false, 8080), mk("etcd", true, 2379))

	if got, err := k.PodProxyTarget(ctx, "ns", "web", 8080); err != nil || got != "http://10.0.0.5:8080" {
		t.Fatalf("declared port should proxy, got %q err=%v", got, err)
	}
	if _, err := k.PodProxyTarget(ctx, "ns", "web", 22); err == nil || HTTPStatus(err) != 403 {
		t.Fatalf("undeclared port must be refused with 403, got %v", err)
	}
	if _, err := k.PodProxyTarget(ctx, "ns", "etcd", 2379); err == nil || HTTPStatus(err) != 403 {
		t.Fatalf("hostNetwork pod must be refused with 403, got %v", err)
	}
}

// The terminal reader goroutine sends resizes while the session tears down.
// The queue must never close its channel (a send on a closed channel panics the
// whole process) and Next must unblock when the session ends.
func TestTerminalSizeQueueSurvivesTeardown(t *testing.T) {
	done := make(chan struct{})
	q := &terminalSizeQueue{ch: make(chan remotecommand.TerminalSize, 4), done: done}
	q.ch <- remotecommand.TerminalSize{Width: 80, Height: 24}
	if s := q.Next(); s == nil || s.Width != 80 {
		t.Fatalf("want queued size, got %v", s)
	}
	close(done)
	if s := q.Next(); s != nil {
		t.Fatalf("Next must return nil after the session ends, got %v", s)
	}
	select { // a late sender must not panic
	case q.ch <- remotecommand.TerminalSize{Width: 1, Height: 1}:
	default:
	}
}
