//go:build kubernetes

package platform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// Node shell (Workstream B5). SECURITY SENSITIVE: a node shell is root on the
// host. It is off unless AXIOM_ENABLE_NODE_SHELL=true, admin-only and blocked for
// namespace-restricted identities (enforced in internal/auth), audited by the
// API layer, and implemented as a short-lived privileged pod pinned to the node
// in a dedicated namespace that is deleted when the session ends or times out.

const (
	nodeShellLabel      = "axiom.zyvor.dev/node-shell"
	nodeShellPodPrefix  = "axiom-node-shell-"
	nodeShellStartWait  = 90 * time.Second
	nodeShellStartPoll  = 500 * time.Millisecond
	nodeShellGraceExtra = 2 * time.Minute
)

// nodeShellStartTimeout is a variable so tests can shorten it.
var nodeShellStartTimeout = nodeShellStartWait

// nodeShellPod builds the privileged, node-pinned pod. nsenter into PID 1's
// namespaces (hostPID) gives a real host shell. Pinning via NodeName bypasses
// the scheduler, so cordoned or tainted nodes can still be reached.
func nodeShellPod(node, user string, timeout time.Duration) *corev1.Pod {
	t, f := true, false
	deadline := int64((timeout + nodeShellGraceExtra).Seconds())
	grace := int64(0)
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodeShellPodPrefix + randHex(4),
			Namespace: NodeShellNamespace(),
			Labels: map[string]string{
				"app.kubernetes.io/name":       "axiom-node-shell",
				"app.kubernetes.io/managed-by": "axiom",
				nodeShellLabel:                 "true",
			},
			Annotations: map[string]string{
				"axiom.zyvor.dev/node":    node,
				"axiom.zyvor.dev/user":    user,
				"axiom.zyvor.dev/timeout": timeout.String(),
			},
		},
		Spec: corev1.PodSpec{
			NodeName:                      node,
			HostPID:                       true,
			HostNetwork:                   true,
			RestartPolicy:                 corev1.RestartPolicyNever,
			ActiveDeadlineSeconds:         &deadline,
			TerminationGracePeriodSeconds: &grace,
			AutomountServiceAccountToken:  &f,
			Tolerations:                   []corev1.Toleration{{Operator: corev1.TolerationOpExists}},
			Containers: []corev1.Container{{
				Name:            "shell",
				Image:           NodeShellImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command:         []string{"sleep", strconv.Itoa(int(deadline))},
				Stdin:           true,
				TTY:             true,
				SecurityContext: &corev1.SecurityContext{Privileged: &t},
			}},
		},
	}
}

// ensureNodeShellNamespace creates the dedicated namespace (labelled for the
// Pod Security "privileged" profile, which node-shell pods require) if missing.
func (k *KubernetesProvider) ensureNodeShellNamespace(ctx context.Context) error {
	name := NodeShellNamespace()
	_, err := k.client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	_, err = k.client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: name,
		Labels: map[string]string{
			"app.kubernetes.io/managed-by":       "axiom",
			"pod-security.kubernetes.io/enforce": "privileged",
			"pod-security.kubernetes.io/audit":   "privileged",
			"pod-security.kubernetes.io/warn":    "privileged",
		},
	}}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

// sweepNodeShellPods deletes finished or overdue node-shell pods, covering the
// case where Axiom crashed mid-session and never ran its deferred cleanup.
func (k *KubernetesProvider) sweepNodeShellPods(ctx context.Context, timeout time.Duration) int {
	list, err := k.client.CoreV1().Pods(NodeShellNamespace()).List(ctx, metav1.ListOptions{LabelSelector: nodeShellLabel + "=true"})
	if err != nil {
		return 0
	}
	n := 0
	for _, p := range list.Items {
		overdue := time.Since(p.CreationTimestamp.Time) > timeout+nodeShellGraceExtra
		done := p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed
		if !overdue && !done {
			continue
		}
		if k.client.CoreV1().Pods(p.Namespace).Delete(ctx, p.Name, metav1.DeleteOptions{}) == nil {
			n++
		}
	}
	return n
}

// prepareNodeShell validates the switch and node, creates the namespace and
// pod, and waits for the container to run. The returned cleanup deletes the pod
// and must always be called. Nothing is left behind when an error is returned.
func (k *KubernetesProvider) prepareNodeShell(ctx context.Context, node, user string, timeout time.Duration) (pod *corev1.Pod, cleanup func(), err error) {
	if !NodeShellEnabled() {
		return nil, func() {}, ErrNodeShellDisabled
	}
	if _, err := k.client.CoreV1().Nodes().Get(ctx, node, metav1.GetOptions{}); err != nil {
		return nil, func() {}, err
	}
	if err := k.ensureNodeShellNamespace(ctx); err != nil {
		return nil, func() {}, fmt.Errorf("preparing node-shell namespace %q: %w", NodeShellNamespace(), err)
	}
	k.sweepNodeShellPods(ctx, timeout)
	created, err := k.client.CoreV1().Pods(NodeShellNamespace()).Create(ctx, nodeShellPod(node, user, timeout), metav1.CreateOptions{})
	if err != nil {
		return nil, func() {}, fmt.Errorf("creating node-shell pod (does the %q namespace enforce a Pod Security profile stricter than privileged?): %w", NodeShellNamespace(), err)
	}
	cleanup = func() {
		// Background context: the request context is usually already cancelled
		// by the time the session ends, which must not leave a privileged pod behind.
		cctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = k.client.CoreV1().Pods(created.Namespace).Delete(cctx, created.Name, metav1.DeleteOptions{})
	}
	deadline := time.Now().Add(nodeShellStartTimeout)
	for {
		p, gerr := k.client.CoreV1().Pods(created.Namespace).Get(ctx, created.Name, metav1.GetOptions{})
		if gerr == nil {
			if p.Status.Phase == corev1.PodRunning {
				return p, cleanup, nil
			}
			if p.Status.Phase == corev1.PodFailed || p.Status.Phase == corev1.PodSucceeded {
				cleanup()
				return nil, func() {}, fmt.Errorf("node-shell pod ended before the shell started (%s)", p.Status.Reason)
			}
			for _, cs := range p.Status.ContainerStatuses {
				if w := cs.State.Waiting; w != nil && (w.Reason == "ErrImagePull" || w.Reason == "ImagePullBackOff") {
					cleanup()
					return nil, func() {}, fmt.Errorf("node-shell image %q cannot be pulled: %s (set AXIOM_NODE_SHELL_IMAGE to an image the node can reach)", NodeShellImage(), w.Reason)
				}
			}
		}
		if time.Now().After(deadline) {
			cleanup()
			return nil, func() {}, NewStatusError(http.StatusGatewayTimeout, errors.New("timed out waiting for the node-shell pod to start"))
		}
		select {
		case <-ctx.Done():
			cleanup()
			return nil, func() {}, ctx.Err()
		case <-time.After(nodeShellStartPoll):
		}
	}
}

// NodeShellWS opens a root shell on the node over a WebSocket. The privileged
// pod is deleted when the socket closes or the session timeout fires. Errors
// before the upgrade are returned as plain HTTP errors by the caller.
func (k *KubernetesProvider) NodeShellWS(ctx context.Context, w http.ResponseWriter, r *http.Request, node, shell, user string) error {
	if !NodeShellEnabled() {
		return ErrNodeShellDisabled
	}
	release, ok := acquireExecWSSession()
	if !ok {
		return NewStatusError(http.StatusTooManyRequests, errors.New("too many open terminal sessions; close one and retry"))
	}
	defer release()
	timeout := NodeShellTimeout()
	pod, cleanup, err := k.prepareNodeShell(ctx, node, user, timeout)
	defer cleanup()
	if err != nil {
		return err
	}
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := []string{"nsenter", "-t", "1", "-m", "-u", "-i", "-n", "-p", "--", shell}
	req := k.client.CoreV1().RESTClient().Post().Resource("pods").Name(pod.Name).Namespace(pod.Namespace).SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{Container: "shell", Command: cmd, Stdin: true, Stdout: true, Stderr: true, TTY: true}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(k.config, http.MethodPost, req.URL())
	if err != nil {
		return NewStatusError(http.StatusBadGateway, err)
	}
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade has already written the HTTP error response.
		return nil
	}
	defer conn.Close()
	conn.SetReadLimit(maxExecWSInput)

	stdinR, stdinW := io.Pipe()
	defer stdinR.Close()
	defer stdinW.Close()
	var writeMu sync.Mutex
	out := lockedWSWriter{mu: &writeMu, conn: conn}
	// The session context carries the hard timeout and is detached from the
	// request context for the same reason ExecWS detaches it.
	streamCtx, streamCancel := context.WithTimeout(context.Background(), timeout)
	defer streamCancel()
	armTerminalIdle(conn, &writeMu, streamCtx.Done(), streamCancel)
	writeMu.Lock()
	_ = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\x1b[33mroot shell on node %s - session ends in %s, after %s idle, or on disconnect; the helper pod is deleted afterwards.\x1b[0m\r\n", node, timeout.Round(time.Second), terminalIdleTimeout.Round(time.Second))))
	writeMu.Unlock()
	resize := &terminalSizeQueue{ch: make(chan remotecommand.TerminalSize, 4), done: streamCtx.Done()}
	readErr := make(chan error, 1)
	go func() {
		for {
			typ, msg, err := conn.ReadMessage()
			if err != nil {
				if ne, ok := err.(interface{ Timeout() bool }); ok && ne.Timeout() {
					writeMu.Lock()
					_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[33mnode shell idle timeout.\x1b[0m\r\n"))
					writeMu.Unlock()
				}
				readErr <- err
				streamCancel()
				return
			}
			touchTerminalIdle(conn)
			if typ != websocket.TextMessage && typ != websocket.BinaryMessage {
				continue
			}
			if bytes.HasPrefix(msg, []byte("__resize__:")) {
				parts := strings.Split(strings.TrimPrefix(string(msg), "__resize__:"), "x")
				if len(parts) == 2 {
					w64, _ := strconv.ParseUint(parts[0], 10, 16)
					h64, _ := strconv.ParseUint(parts[1], 10, 16)
					select {
					case resize.ch <- remotecommand.TerminalSize{Width: uint16(w64), Height: uint16(h64)}:
					default:
					}
				}
				continue
			}
			if _, err := stdinW.Write(msg); err != nil {
				readErr <- err
				streamCancel()
				return
			}
		}
	}()
	execErr := make(chan error, 1)
	go func() {
		execErr <- executor.StreamWithContext(streamCtx, remotecommand.StreamOptions{Stdin: stdinR, Stdout: out, Stderr: out, Tty: true, TerminalSizeQueue: resize})
	}()
	select {
	case err := <-execErr:
		if err != nil {
			writeMu.Lock()
			if errors.Is(streamCtx.Err(), context.DeadlineExceeded) {
				_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[33mnode shell session timed out.\x1b[0m\r\n"))
			} else {
				_ = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\n\x1b[31mexec failed:\x1b[0m %s\r\n", err.Error())))
			}
			writeMu.Unlock()
		}
		return nil
	case <-readErr:
		return nil
	}
}
