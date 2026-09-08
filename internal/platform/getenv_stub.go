//go:build !kubernetes

package platform

func getenvOS(k string) string { return "" }
