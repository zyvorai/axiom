package platform

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

// CreateDebugContainer simulates adding an ephemeral debug container. The demo
// has no kubelet, so nothing runs and no terminal can attach; the response says
// so rather than pretending otherwise.
func (d *DemoProvider) CreateDebugContainer(ctx context.Context, ns, pod string, req DebugRequest) (DebugContainer, error) {
	pods, err := d.ListPods(ctx, ns)
	if err != nil {
		return DebugContainer{}, err
	}
	for _, p := range pods {
		if p.Name != pod {
			continue
		}
		if req.Target != "" {
			found := false
			for _, c := range p.Containers {
				if c == req.Target {
					found = true
				}
			}
			if !found {
				return DebugContainer{}, notFoundf("target container %q not found in pod %s/%s", req.Target, ns, pod)
			}
		}
		return DebugContainer{
			Namespace: ns, Pod: pod, Container: newDebugContainerName(), Image: DebugImage(), Target: req.Target,
			Ready:   false,
			Message: "Demo mode: the ephemeral container is simulated and no terminal can attach. Connect Axiom to a live cluster to use it.",
		}, nil
	}
	return DebugContainer{}, notFoundf("pod %s/%s not found", ns, pod)
}

// WorkloadLogs returns synthetic, timestamped logs merged across the
// workload's pods, using the same merge as the Kubernetes provider.
func (d *DemoProvider) WorkloadLogs(ctx context.Context, ns, kind, name string, opts WorkloadLogOptions) (io.ReadCloser, error) {
	if !SupportedLogWorkloadKind(kind) {
		return nil, NewStatusError(400, fmt.Errorf("log aggregation is not supported for kind %q", kind))
	}
	d.mu.RLock()
	_, ok := d.workloads[key(ns, kind, name)]
	d.mu.RUnlock()
	if !ok {
		return nil, notFoundf("workload %s/%s/%s not found", ns, kind, name)
	}
	pods, _ := d.ListPods(ctx, ns)
	tail := int(opts.tail())
	now := time.Now().UTC()
	var streams []labeledLogStream
	i := 0
	for _, p := range pods {
		if p.WorkloadID != name {
			continue
		}
		containers := p.Containers
		if opts.Container != "" {
			containers = nil
			for _, c := range p.Containers {
				if c == opts.Container {
					containers = append(containers, c)
				}
			}
		}
		for _, c := range containers {
			if len(streams) >= MaxWorkloadLogStreams {
				break
			}
			crashing := p.Status == "CrashLoopBackOff"
			if opts.Previous && !crashing {
				continue // only a restarted container has a previous instance
			}
			var lines []string
			for n := 0; n < 4; n++ {
				ts := now.Add(-time.Duration(40-n*10+i) * time.Second).Format(time.RFC3339Nano)
				switch {
				case crashing && n == 3:
					lines = append(lines, ts+" ERROR collector exited: failed to start receiver otlp: bind: address already in use")
				case n == 2:
					lines = append(lines, ts+" WARN upstream latency service=postgres duration=74ms")
				default:
					lines = append(lines, ts+fmt.Sprintf(" INFO request complete status=200 duration=%dms", 12+i+n))
				}
			}
			if len(lines) > tail {
				lines = lines[len(lines)-tail:]
			}
			streams = append(streams, labeledLogStream{
				label: p.Name + "/" + c,
				rc:    io.NopCloser(strings.NewReader(strings.Join(lines, "\n") + "\n")),
			})
			i++
		}
	}
	if len(streams) == 0 {
		if opts.Previous {
			return nil, notFoundf("no previous terminated container found for %s/%s", ns, name)
		}
		return nil, notFoundf("workload %s/%s has no pods to read logs from", ns, name)
	}
	// The demo has no live source, so follow behaves like a one-shot read.
	return mergeLogStreams(streams, false), nil
}
