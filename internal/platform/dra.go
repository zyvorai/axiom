package platform

import "context"

// DRAObject is one resource.k8s.io object for the DRA browser.
type DRAObject struct {
	Kind      string `json:"kind"` // DeviceClass | ResourceSlice | ResourceClaim | ResourceClaimTemplate
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	Driver    string `json:"driver,omitempty"`
	Status    string `json:"status,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Age       string `json:"age,omitempty"`
}

// DRAInventory is GET /api/v1/dra.
type DRAInventory struct {
	Available bool        `json:"available"`
	Note      string      `json:"note,omitempty"`
	Objects   []DRAObject `json:"objects"`
	Counts    map[string]int `json:"counts"`
}

// ListDRA returns a read-only DRA inventory when resource.k8s.io is present.
func ListDRA(ctx context.Context, p Provider) (DRAInventory, error) {
	if sp, ok := p.(*SwitchableProvider); ok {
		return sp.ListDRA(ctx)
	}
	if l, ok := p.(interface {
		ListDRA(context.Context) (DRAInventory, error)
	}); ok {
		return l.ListDRA(ctx)
	}
	return DRAInventory{Available: false, Note: "DRA browser not available on this provider", Objects: []DRAObject{}, Counts: map[string]int{}}, nil
}

func (d *DemoProvider) ListDRA(ctx context.Context) (DRAInventory, error) {
	return DRAInventory{
		Available: true,
		Note:      "Demo DRA inventory (simulated DeviceClass + claim)",
		Objects: []DRAObject{
			{Kind: "DeviceClass", Name: "gpu.nvidia.com", Driver: "gpu.nvidia.com", Status: "Available", Detail: "1 selector(s)", Age: "30d"},
			{Kind: "ResourceSlice", Name: "gpu-01-slice", Driver: "gpu.nvidia.com", Status: "Published", Detail: "node=gpu-01", Age: "17d"},
			{Kind: "ResourceClaim", Namespace: "ml", Name: "training-gpu", Driver: "gpu.nvidia.com", Status: "Allocated", Detail: "devices=gpu-0 · pod=train-0", Age: "2d"},
		},
		Counts: map[string]int{"DeviceClass": 1, "ResourceSlice": 1, "ResourceClaim": 1},
	}, nil
}
