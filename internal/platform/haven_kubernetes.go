//go:build kubernetes

package platform

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	identityPlaneGVR = schema.GroupVersionResource{
		Group: "identity.haven.dev", Version: "v1alpha1", Resource: "identityplanes",
	}
	keycloakGVRs = []schema.GroupVersionResource{
		{Group: "k8s.keycloak.org", Version: "v2beta1", Resource: "keycloaks"},
		{Group: "k8s.keycloak.org", Version: "v2alpha1", Resource: "keycloaks"},
	}
	cnpgClusterGVR = schema.GroupVersionResource{
		Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters",
	}
)

func (k *KubernetesProvider) GetHavenIdentity(ctx context.Context, ns string) (HavenIdentitySummary, error) {
	out := HavenIdentitySummary{
		Planes:    []IdentityPlane{},
		Keycloaks: []HavenKeycloak{},
		Databases: []HavenDatabase{},
		OIDC: HavenOIDCHints{
			ClientID: "axiom",
			Note:     "Set AXIOM_OIDC_ISSUER / AXIOM_OIDC_LOGIN_URL once Keycloak is Ready.",
		},
	}
	if k.dynamic == nil {
		out.Message = "dynamic client unavailable"
		return out, nil
	}

	planeNS := ns
	if planeNS == "" || planeNS == "identity" {
		planeNS = "identity"
	}
	// Keycloak Operator v26 watches only its install namespace by default.
	listNS := "all"

	planes, err := k.ListIdentityPlanes(ctx, planeNS)
	if err != nil {
		return out, err
	}
	out.Planes = planes

	kcs, err := k.ListHavenKeycloaks(ctx, listNS)
	if err != nil {
		return out, err
	}
	out.Keycloaks = kcs

	dbs, err := k.ListHavenDatabases(ctx, listNS)
	if err != nil {
		return out, err
	}
	// Prefer Haven-labeled / identity DB when many CNPG clusters exist.
	filtered := make([]HavenDatabase, 0, len(dbs))
	for _, d := range dbs {
		if d.Namespace == "identity" || strings.Contains(d.Name, "platform") {
			filtered = append(filtered, d)
		}
	}
	if len(filtered) > 0 {
		out.Databases = filtered
	} else {
		out.Databases = dbs
	}

	adminNS := "keycloak"
	adminName := "platform"
	if len(kcs) > 0 {
		adminNS = kcs[0].Namespace
		adminName = kcs[0].Name
	}
	admin, _ := k.GetHavenAdminBootstrap(ctx, adminNS, adminName)
	out.Admin = &admin

	readyKC := false
	for _, kc := range kcs {
		if !kc.Ready {
			continue
		}
		readyKC = true
		host := kc.Hostname
		if host == "" {
			continue
		}
		base := "http://" + host
		if kc.URL != "" {
			base = strings.TrimRight(kc.URL, "/")
		}
		out.OIDC.Issuer = base + "/realms/platform"
		out.OIDC.LoginURL = out.OIDC.Issuer + "/protocol/openid-connect/auth"
		break
	}
	dbReady := false
	for _, d := range out.Databases {
		if d.Ready {
			dbReady = true
			break
		}
	}
	out.Ready = readyKC && dbReady
	if out.Ready {
		out.Message = "Haven plane workloads ready"
		out.OIDC.Note = "Copy issuer/login into Settings → Integrations when ready."
	} else if len(kcs) == 0 && len(out.Databases) == 0 && len(planes) == 0 {
		out.Message = "No Haven resources — apply deploy/haven or Haven compose path"
	} else {
		out.Message = "Plane provisioning — wait for CNPG Ready and Keycloak pods"
	}
	return out, nil
}

func (k *KubernetesProvider) ListIdentityPlanes(ctx context.Context, ns string) ([]IdentityPlane, error) {
	if k.dynamic == nil {
		return []IdentityPlane{}, nil
	}
	list, err := k.dynamic.Resource(identityPlaneGVR).Namespace(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isMissingCRD(err) {
			return []IdentityPlane{}, nil
		}
		return nil, err
	}
	out := make([]IdentityPlane, 0, len(list.Items))
	for _, item := range list.Items {
		profile, _, _ := unstructured.NestedString(item.Object, "spec", "profile")
		host, _, _ := unstructured.NestedString(item.Object, "spec", "hostname")
		console, _, _ := unstructured.NestedString(item.Object, "spec", "consoleHostname")
		phase, _, _ := unstructured.NestedString(item.Object, "status", "phase")
		if phase == "" {
			phase = "Pending"
		}
		msg, _, _ := unstructured.NestedString(item.Object, "status", "message")
		kcURL, _, _ := unstructured.NestedString(item.Object, "status", "endpoints", "keycloak")
		adminURL, _, _ := unstructured.NestedString(item.Object, "status", "endpoints", "admin")
		issuer := ""
		if host != "" {
			issuer = "http://" + host + "/realms/platform"
		}
		out = append(out, IdentityPlane{
			Namespace: item.GetNamespace(), Name: item.GetName(),
			Profile: profile, Hostname: host, ConsoleHostname: console,
			Phase: phase, Message: msg, KeycloakURL: kcURL, AdminURL: adminURL,
			IssuerURL: issuer, Age: age(item.GetCreationTimestamp().Time),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace == out[j].Namespace {
			return out[i].Name < out[j].Name
		}
		return out[i].Namespace < out[j].Namespace
	})
	return out, nil
}

func (k *KubernetesProvider) ListHavenKeycloaks(ctx context.Context, ns string) ([]HavenKeycloak, error) {
	if k.dynamic == nil {
		return []HavenKeycloak{}, nil
	}
	list, _, err := k.listFirstGVR(ctx, keycloakGVRs, ns)
	if err != nil {
		if isMissingCRD(err) {
			return []HavenKeycloak{}, nil
		}
		return nil, err
	}
	out := make([]HavenKeycloak, 0, len(list.Items))
	for _, item := range list.Items {
		inst, _, _ := unstructured.NestedInt64(item.Object, "spec", "instances")
		host, _, _ := unstructured.NestedString(item.Object, "spec", "hostname", "hostname")
		ready := false
		msg := ""
		if conds, ok, _ := unstructured.NestedSlice(item.Object, "status", "conditions"); ok {
			for _, c := range conds {
				cm, ok := c.(map[string]any)
				if !ok {
					continue
				}
				typ, _ := cm["type"].(string)
				st, _ := cm["status"].(string)
				if typ == "Ready" && strings.EqualFold(st, "True") {
					ready = true
				}
				if typ == "Ready" {
					msg, _ = cm["message"].(string)
				}
			}
		}
		if !ready {
			if st, ok, _ := unstructured.NestedBool(item.Object, "status", "ready"); ok {
				ready = st
			}
		}
		url := ""
		if host != "" {
			url = "http://" + host
		}
		out = append(out, HavenKeycloak{
			Namespace: item.GetNamespace(), Name: item.GetName(),
			Ready: ready, Instances: inst, Hostname: host, URL: url, Message: msg,
			Age: age(item.GetCreationTimestamp().Time),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace == out[j].Namespace {
			return out[i].Name < out[j].Name
		}
		return out[i].Namespace < out[j].Namespace
	})
	return out, nil
}

func (k *KubernetesProvider) ListHavenDatabases(ctx context.Context, ns string) ([]HavenDatabase, error) {
	if k.dynamic == nil {
		return []HavenDatabase{}, nil
	}
	list, err := k.dynamic.Resource(cnpgClusterGVR).Namespace(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isMissingCRD(err) {
			return []HavenDatabase{}, nil
		}
		return nil, err
	}
	out := make([]HavenDatabase, 0, len(list.Items))
	for _, item := range list.Items {
		inst, _, _ := unstructured.NestedInt64(item.Object, "spec", "instances")
		phase, _, _ := unstructured.NestedString(item.Object, "status", "phase")
		ready := false
		if conds, ok, _ := unstructured.NestedSlice(item.Object, "status", "conditions"); ok {
			for _, c := range conds {
				cm, ok := c.(map[string]any)
				if !ok {
					continue
				}
				if typ, _ := cm["type"].(string); typ == "Ready" {
					if st, _ := cm["status"].(string); strings.EqualFold(st, "True") {
						ready = true
					}
				}
			}
		}
		if phase == "" {
			phase = "Unknown"
		}
		out = append(out, HavenDatabase{
			Namespace: item.GetNamespace(), Name: item.GetName(),
			Ready: ready, Instances: inst, Status: phase,
			Age: age(item.GetCreationTimestamp().Time),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace == out[j].Namespace {
			return out[i].Name < out[j].Name
		}
		return out[i].Namespace < out[j].Namespace
	})
	return out, nil
}

func (k *KubernetesProvider) GetHavenAdminBootstrap(ctx context.Context, ns, plane string) (HavenAdminBootstrap, error) {
	out := HavenAdminBootstrap{
		Namespace: ns,
		Hint:      "kubectl get secret <name> -n identity -o jsonpath='{.data.password}' | base64 -d",
	}
	if k.client == nil {
		return out, nil
	}
	candidates := []string{plane + "-initial-admin", plane + "-bootstrap"}
	for _, name := range candidates {
		sec, err := k.client.CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return out, err
		}
		out.SecretName = name
		out.Exists = true
		out.UsernameKey = pickSecretKey(sec, "username", "user", "admin")
		out.PasswordKey = pickSecretKey(sec, "password", "pass")
		out.Hint = fmt.Sprintf("kubectl -n %s get secret %s -o jsonpath='{.data.%s}' | base64 -d", ns, name, out.PasswordKey)
		return out, nil
	}
	out.SecretName = plane + "-initial-admin"
	out.Hint = "Bootstrap secret appears after Keycloak CR is Ready"
	return out, nil
}

func pickSecretKey(sec *corev1.Secret, prefer ...string) string {
	for _, p := range prefer {
		if _, ok := sec.Data[p]; ok {
			return p
		}
	}
	for k := range sec.Data {
		return k
	}
	return "password"
}
