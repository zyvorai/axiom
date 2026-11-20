//go:build kubernetes

package platform

import corev1 "k8s.io/api/core/v1"

// podRefs lists the PVCs, ConfigMaps and Secrets a pod mounts or references
// (volumes, env, envFrom, imagePullSecrets). Names only; no values are read.
func podRefs(p corev1.Pod) (pvcs, cms, secs []string) {
	for _, v := range p.Spec.Volumes {
		if v.PersistentVolumeClaim != nil {
			pvcs = append(pvcs, v.PersistentVolumeClaim.ClaimName)
		}
		if v.ConfigMap != nil {
			cms = append(cms, v.ConfigMap.Name)
		}
		if v.Secret != nil {
			secs = append(secs, v.Secret.SecretName)
		}
		if v.Projected != nil {
			for _, s := range v.Projected.Sources {
				if s.ConfigMap != nil {
					cms = append(cms, s.ConfigMap.Name)
				}
				if s.Secret != nil {
					secs = append(secs, s.Secret.Name)
				}
			}
		}
	}
	conts := append(append([]corev1.Container{}, p.Spec.InitContainers...), p.Spec.Containers...)
	for _, c := range conts {
		for _, ef := range c.EnvFrom {
			if ef.ConfigMapRef != nil {
				cms = append(cms, ef.ConfigMapRef.Name)
			}
			if ef.SecretRef != nil {
				secs = append(secs, ef.SecretRef.Name)
			}
		}
		for _, e := range c.Env {
			if e.ValueFrom == nil {
				continue
			}
			if e.ValueFrom.ConfigMapKeyRef != nil {
				cms = append(cms, e.ValueFrom.ConfigMapKeyRef.Name)
			}
			if e.ValueFrom.SecretKeyRef != nil {
				secs = append(secs, e.ValueFrom.SecretKeyRef.Name)
			}
		}
	}
	for _, s := range p.Spec.ImagePullSecrets {
		secs = append(secs, s.Name)
	}
	return pvcs, cms, secs
}
