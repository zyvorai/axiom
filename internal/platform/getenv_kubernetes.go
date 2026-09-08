//go:build kubernetes

package platform

import "os"

func getenvOS(k string) string { return os.Getenv(k) }
