//go:build kubernetes

package platform

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// debugContainerWait is how long CreateDebugContainer waits for the new
// ephemeral container to start before returning with Ready=false. A variable so
// tests (whose fake pods never start) can turn the wait off.
var debugContainerWait = 25 * time.Second

const debugContainerPoll = 500 * time.Millisecond

// debugContainerSpec builds the ephemeral container. It runs `sleep` so the
// terminal can exec a shell into it (rather than attaching to PID 1), and carries
// a securityContext that satisfies the Pod Security "restricted" profile —
// ephemeral containers are admitted against the pod's namespace policy too.
func debugContainerSpec(name, image, target string) corev1.EphemeralContainer {
	f, t := false, true
	uid := int64(65534)
	return corev1.EphemeralContainer{
		EphemeralContainerCommon: corev1.EphemeralContainerCommon{
			Name:            name,
			Image:           image,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command:         []string{"sleep", "3600"},
			Stdin:           true,
			TTY:             true,
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: &f,
				RunAsNonRoot:             &t,
				RunAsUser:                &uid,
				Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
				SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
			},
		},
		TargetContainerName: target,
	}
}

// CreateDebugContainer adds an ephemeral debug container to a running pod via
// the pods/ephemeralcontainers subresource and waits briefly for it to start.
// Ephemeral containers can never be removed from a pod; it disappears with the pod.
func (k *KubernetesProvider) CreateDebugContainer(ctx context.Context, ns, pod string, req DebugRequest) (DebugContainer, error) {
	p, err := k.client.CoreV1().Pods(ns).Get(ctx, pod, metav1.GetOptions{})
	if err != nil {
		return DebugContainer{}, err
	}
	if p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed {
		return DebugContainer{}, NewStatusError(http.StatusConflict, fmt.Errorf("pod %s/%s has terminated; a debug container needs a running pod", ns, pod))
	}
	if p.DeletionTimestamp != nil {
		return DebugContainer{}, NewStatusError(http.StatusConflict, fmt.Errorf("pod %s/%s is being deleted", ns, pod))
	}
	if t := strings.TrimSpace(req.Target); t != "" {
		found := false
		for _, c := range p.Spec.Containers {
			found = found || c.Name == t
		}
		for _, c := range p.Spec.InitContainers {
			found = found || c.Name == t
		}
		if !found {
			return DebugContainer{}, notFoundf("target container %q not found in pod %s/%s", t, ns, pod)
		}
		req.Target = t
	}
	name, image := newDebugContainerName(), DebugImage()
	updated := p.DeepCopy()
	updated.Spec.EphemeralContainers = append(updated.Spec.EphemeralContainers, debugContainerSpec(name, image, req.Target))
	if _, err := k.client.CoreV1().Pods(ns).UpdateEphemeralContainers(ctx, pod, updated, metav1.UpdateOptions{}); err != nil {
		return DebugContainer{}, err
	}
	out := DebugContainer{Namespace: ns, Pod: pod, Container: name, Image: image, Target: req.Target}
	out.Ready, out.Message = k.waitDebugContainer(ctx, ns, pod, name)
	return out, nil
}

func (k *KubernetesProvider) waitDebugContainer(ctx context.Context, ns, pod, name string) (bool, string) {
	if debugContainerWait <= 0 {
		return false, "The debug container was added and is starting."
	}
	deadline := time.Now().Add(debugContainerWait)
	msg := "The debug container was added and is starting."
	for {
		p, err := k.client.CoreV1().Pods(ns).Get(ctx, pod, metav1.GetOptions{})
		if err == nil {
			for _, cs := range p.Status.EphemeralContainerStatuses {
				if cs.Name != name {
					continue
				}
				switch {
				case cs.State.Running != nil:
					return true, "The debug container is running."
				case cs.State.Waiting != nil && (cs.State.Waiting.Reason == "ErrImagePull" || cs.State.Waiting.Reason == "ImagePullBackOff" || cs.State.Waiting.Reason == "InvalidImageName"):
					return false, fmt.Sprintf("The debug container cannot start: %s %s. Check AXIOM_DEBUG_IMAGE and registry access.", cs.State.Waiting.Reason, cs.State.Waiting.Message)
				case cs.State.Waiting != nil && cs.State.Waiting.Message != "":
					msg = cs.State.Waiting.Reason + ": " + cs.State.Waiting.Message
				case cs.State.Terminated != nil:
					return false, fmt.Sprintf("The debug container exited (%s, code %d).", cs.State.Terminated.Reason, cs.State.Terminated.ExitCode)
				}
			}
		}
		if time.Now().After(deadline) {
			return false, msg
		}
		select {
		case <-ctx.Done():
			return false, msg
		case <-time.After(debugContainerPoll):
		}
	}
}
