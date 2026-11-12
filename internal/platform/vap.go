package platform

import (
	"context"
	"sort"
)

// VAPSummary is a ValidatingAdmissionPolicy (native Kubernetes admission).
type VAPSummary struct {
	Name       string   `json:"name"`
	Kind       string   `json:"kind"` // ValidatingAdmissionPolicy | ValidatingAdmissionPolicyBinding
	ParamKind  string   `json:"paramKind,omitempty"`
	Validations int     `json:"validations,omitempty"`
	Bindings   []string `json:"bindings,omitempty"`
	Age        string   `json:"age,omitempty"`
	Status     string   `json:"status,omitempty"`
}

// VAPReport is GET /api/v1/admission-policies.
type VAPReport struct {
	Available bool         `json:"available"`
	Note      string       `json:"note,omitempty"`
	Policies  []VAPSummary `json:"policies"`
}

// ListValidatingAdmissionPolicies returns native VAP objects when present.
func ListValidatingAdmissionPolicies(ctx context.Context, p Provider) (VAPReport, error) {
	if l, ok := p.(interface {
		ListValidatingAdmissionPolicies(context.Context) (VAPReport, error)
	}); ok {
		return l.ListValidatingAdmissionPolicies(ctx)
	}
	return VAPReport{Available: false, Note: "not available", Policies: []VAPSummary{}}, nil
}

func (d *DemoProvider) ListValidatingAdmissionPolicies(ctx context.Context) (VAPReport, error) {
	return VAPReport{
		Available: true,
		Note:      "Demo ValidatingAdmissionPolicy inventory",
		Policies: []VAPSummary{
			{Name: "require-labels", Kind: "ValidatingAdmissionPolicy", Validations: 1, Status: "Active", Age: "14d",
				Bindings: []string{"require-labels-binding"}},
			{Name: "require-labels-binding", Kind: "ValidatingAdmissionPolicyBinding", Age: "14d", Status: "Active"},
		},
	}, nil
}

func sortVAP(p []VAPSummary) {
	sort.Slice(p, func(i, j int) bool {
		if p[i].Kind == p[j].Kind {
			return p[i].Name < p[j].Name
		}
		return p[i].Kind < p[j].Kind
	})
}
