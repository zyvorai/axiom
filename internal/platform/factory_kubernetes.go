//go:build kubernetes

package platform

func NewProvider(mode, kubeconfig string) (Provider, error) {
	if mode == "demo" {
		return NewDemoProvider(), nil
	}
	return NewKubernetesProvider(kubeconfig)
}
