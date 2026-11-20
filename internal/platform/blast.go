package platform

import (
	"context"
	"fmt"
	"strings"
)

type VolumeLister interface {
	WorkloadVolumes(context.Context, string, string) ([]string, error)
}

func StillPreview(ctx context.Context, p Provider, ns, kind, name string, action StillAction, podName string) (BlastPreview, error) {
	workloads, err := p.ListWorkloads(ctx, ns)
	if err != nil {
		return BlastPreview{}, err
	}
	var w *Workload
	for i := range workloads {
		if workloads[i].Namespace == ns && strings.EqualFold(workloads[i].Kind, kind) && workloads[i].Name == name {
			w = &workloads[i]
			break
		}
	}
	if w == nil {
		return BlastPreview{}, fmt.Errorf("workload not found")
	}
	pods, err := p.ListPods(ctx, ns)
	if err != nil {
		return BlastPreview{}, err
	}
	services, err := p.ListServices(ctx, ns)
	if err != nil {
		return BlastPreview{}, err
	}
	prefix := w.Name + "-"
	names := make([]string, 0)
	ports := make([]string, 0)
	for _, pod := range pods {
		if pod.Namespace != ns {
			continue
		}
		// Exact ownership only — never substring (web must not match web-api-*).
		owned := pod.WorkloadID == w.Name || strings.HasPrefix(pod.Name, prefix) || pod.Name == w.Name
		if !owned {
			continue
		}
		names = append(names, pod.Name)
		for _, port := range pod.Ports {
			ports = append(ports, fmt.Sprintf("%d/%s", port.ContainerPort, port.Protocol))
		}
	}
	targetNames := names
	if action == StillSleep {
		if podName != "" {
			targetNames = []string{podName}
		} else if len(names) > 0 {
			targetNames = []string{names[0]}
		} else {
			targetNames = nil
		}
	}
	svcCount := 0
	rps := 0
	for _, s := range services {
		if s.Namespace != ns {
			continue
		}
		for _, ep := range s.Endpoints {
			for _, n := range names {
				if ep == n {
					svcCount++
					for _, sp := range s.Ports {
						rps += int(sp.Port)
					}
					break
				}
			}
		}
	}
	atlas := []string{}
	if vl, ok := p.(VolumeLister); ok {
		if vols, err := vl.WorkloadVolumes(ctx, ns, w.Name); err == nil {
			atlas = vols
		}
	}
	kindStr := string(action)
	if kindStr == "" {
		kindStr = "hush"
	}
	replicas := len(names)
	if action == StillSleep {
		replicas = len(targetNames)
	}
	return BlastPreview{
		Kind: kindStr, WorkloadID: w.Name, Namespace: ns, Replicas: replicas,
		Sessions: CountPresence(names, ns), Windows: CountWindows(names, ns),
		Services: svcCount, Rps: rps, Names: targetNames, Ports: ports, Atlas: atlas,
	}, nil
}
