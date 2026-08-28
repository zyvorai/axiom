//go:build !kubernetes

package platform

import "fmt"

func NewProvider(mode, kubeconfig string) (Provider, error) {
	if mode != "demo" {
		return nil, fmt.Errorf("mode %q requires a production build: go build -tags kubernetes", mode)
	}
	return NewDemoProvider(), nil
}
