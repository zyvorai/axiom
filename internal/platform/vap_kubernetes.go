//go:build kubernetes

package platform

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func (k *KubernetesProvider) ListValidatingAdmissionPolicies(ctx context.Context) (VAPReport, error) {
	rep := VAPReport{Policies: []VAPSummary{}}
	if k.dynamic == nil {
		rep.Note = "dynamic client unavailable"
		return rep, nil
	}
	policyGVR := schema.GroupVersionResource{Group: "admissionregistration.k8s.io", Version: "v1", Resource: "validatingadmissionpolicies"}
	bindingGVR := schema.GroupVersionResource{Group: "admissionregistration.k8s.io", Version: "v1", Resource: "validatingadmissionpolicybindings"}
	plist, err := k.dynamic.Resource(policyGVR).List(ctx, metav1.ListOptions{Limit: 200})
	if err != nil {
		rep.Note = "ValidatingAdmissionPolicy API not available"
		return rep, nil
	}
	rep.Available = true
	bindingsByPolicy := map[string][]string{}
	if blist, err := k.dynamic.Resource(bindingGVR).List(ctx, metav1.ListOptions{Limit: 200}); err == nil {
		for _, b := range blist.Items {
			pol, _, _ := unstructured.NestedString(b.Object, "spec", "policyName")
			if pol != "" {
				bindingsByPolicy[pol] = append(bindingsByPolicy[pol], b.GetName())
			}
			rep.Policies = append(rep.Policies, VAPSummary{
				Name: b.GetName(), Kind: "ValidatingAdmissionPolicyBinding",
				Age: age(b.GetCreationTimestamp().Time), Status: "Active",
			})
		}
	}
	for _, it := range plist.Items {
		vals, _, _ := unstructured.NestedSlice(it.Object, "spec", "validations")
		param, _, _ := unstructured.NestedString(it.Object, "spec", "paramKind", "kind")
		rep.Policies = append(rep.Policies, VAPSummary{
			Name: it.GetName(), Kind: "ValidatingAdmissionPolicy",
			ParamKind: param, Validations: len(vals),
			Bindings: bindingsByPolicy[it.GetName()],
			Age:      age(it.GetCreationTimestamp().Time), Status: "Active",
		})
	}
	sortVAP(rep.Policies)
	return rep, nil
}
