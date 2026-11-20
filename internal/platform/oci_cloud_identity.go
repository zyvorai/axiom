package platform

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// OCICloudProvider names a registry cloud-identity helper.
const (
	OCICloudECR = "ecr"
	OCICloudGAR = "gar"
	OCICloudACR = "acr"
)

// RefreshOCICloudToken tries to obtain a short-lived registry password via the
// local cloud CLI (aws / gcloud / az). Returns the token and a note; never
// stores the token itself — caller decides whether to write a credential.
func RefreshOCICloudToken(ctx context.Context, provider, registry string) (token, note string, err error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	registry = strings.TrimSpace(registry)
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	switch provider {
	case OCICloudECR:
		// aws ecr get-login-password --region <from registry host>
		region := ecrRegionFromRegistry(registry)
		args := []string{"ecr", "get-login-password"}
		if region != "" {
			args = append(args, "--region", region)
		}
		out, err := exec.CommandContext(ctx, "aws", args...).Output()
		if err != nil {
			return "", "", fmt.Errorf("aws ecr get-login-password failed (install AWS CLI and configure credentials): %w", err)
		}
		return strings.TrimSpace(string(out)), "ECR password via AWS CLI; paste into registry credential or rotate soon", nil
	case OCICloudGAR, "gcr", "artifactregistry":
		out, err := exec.CommandContext(ctx, "gcloud", "auth", "print-access-token").Output()
		if err != nil {
			return "", "", fmt.Errorf("gcloud auth print-access-token failed (install gcloud and authenticate): %w", err)
		}
		return strings.TrimSpace(string(out)), "GAR/GCR token via gcloud; username is often oauth2accesstoken", nil
	case OCICloudACR:
		// az acr login --name <name> --expose-token
		name := acrNameFromRegistry(registry)
		if name == "" {
			return "", "", fmt.Errorf("could not derive ACR name from registry %q", registry)
		}
		out, err := exec.CommandContext(ctx, "az", "acr", "login", "--name", name, "--expose-token", "-o", "tsv", "--query", "accessToken").Output()
		if err != nil {
			return "", "", fmt.Errorf("az acr login --expose-token failed (install Azure CLI): %w", err)
		}
		return strings.TrimSpace(string(out)), "ACR token via Azure CLI; username is often 00000000-0000-0000-0000-000000000000", nil
	default:
		return "", "", fmt.Errorf("unknown cloud provider %q (want ecr, gar, or acr)", provider)
	}
}

func ecrRegionFromRegistry(host string) string {
	// 123.dkr.ecr.us-east-1.amazonaws.com
	host = strings.ToLower(host)
	parts := strings.Split(host, ".")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "ecr" {
			return parts[i+1]
		}
	}
	return ""
}

func acrNameFromRegistry(host string) string {
	// myregistry.azurecr.io
	host = strings.ToLower(strings.TrimSpace(host))
	if i := strings.Index(host, ".azurecr.io"); i > 0 {
		return host[:i]
	}
	return ""
}
