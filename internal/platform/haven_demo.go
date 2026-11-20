package platform

import "context"

func (d *DemoProvider) GetHavenIdentity(ctx context.Context, ns string) (HavenIdentitySummary, error) {
	planes, _ := d.ListIdentityPlanes(ctx, ns)
	kcs, _ := d.ListHavenKeycloaks(ctx, ns)
	dbs, _ := d.ListHavenDatabases(ctx, ns)
	admin, _ := d.GetHavenAdminBootstrap(ctx, "identity", "platform")
	return HavenIdentitySummary{
		Planes: planes, Keycloaks: kcs, Databases: dbs, Admin: &admin,
		OIDC: HavenOIDCHints{
			Issuer: "http://auth.example.local/realms/platform",
			LoginURL: "http://auth.example.local/realms/platform/protocol/openid-connect/auth",
			ClientID: "axiom", Configured: false,
			Note: "Demo — configure AXIOM_OIDC_* against a live Haven plane",
		},
		Ready: true, Message: "Demo Haven plane",
	}, nil
}

func (d *DemoProvider) ListIdentityPlanes(ctx context.Context, ns string) ([]IdentityPlane, error) {
	all := []IdentityPlane{{
		Namespace: "identity", Name: "platform", Profile: "dev",
		Hostname: "auth.example.local", ConsoleHostname: "haven.example.local",
		Phase: "Ready", KeycloakURL: "http://auth.example.local",
		AdminURL: "http://auth.example.local/admin", IssuerURL: "http://auth.example.local/realms/platform",
		Age: "1d",
	}}
	return filterNS(ns, all, func(p IdentityPlane) string { return p.Namespace }), nil
}

func (d *DemoProvider) ListHavenKeycloaks(ctx context.Context, ns string) ([]HavenKeycloak, error) {
	all := []HavenKeycloak{{
		Namespace: "identity", Name: "platform", Ready: true, Instances: 1,
		Hostname: "auth.example.local", URL: "http://auth.example.local", Age: "1d",
	}}
	return filterNS(ns, all, func(k HavenKeycloak) string { return k.Namespace }), nil
}

func (d *DemoProvider) ListHavenDatabases(ctx context.Context, ns string) ([]HavenDatabase, error) {
	all := []HavenDatabase{{
		Namespace: "identity", Name: "platform-db", Ready: true, Instances: 1, Status: "Cluster in healthy state", Age: "1d",
	}}
	return filterNS(ns, all, func(d HavenDatabase) string { return d.Namespace }), nil
}

func (d *DemoProvider) GetHavenAdminBootstrap(ctx context.Context, ns, plane string) (HavenAdminBootstrap, error) {
	return HavenAdminBootstrap{
		Namespace: ns, SecretName: plane + "-initial-admin", Exists: true,
		UsernameKey: "username", PasswordKey: "password",
		Hint: "demo mode — no live secret",
	}, nil
}
