package platform

// IdentityPlane is Haven's IdentityPlane CR (identity.haven.dev).
type IdentityPlane struct {
	Namespace       string `json:"namespace"`
	Name            string `json:"name"`
	Profile         string `json:"profile"`
	Hostname        string `json:"hostname"`
	ConsoleHostname string `json:"consoleHostname,omitempty"`
	Phase           string `json:"phase"`
	Message         string `json:"message,omitempty"`
	KeycloakURL     string `json:"keycloakURL,omitempty"`
	AdminURL        string `json:"adminURL,omitempty"`
	IssuerURL       string `json:"issuerURL,omitempty"`
	Age             string `json:"age"`
}

// HavenKeycloak is a Keycloak CR summary for the Identity tab.
type HavenKeycloak struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Ready     bool   `json:"ready"`
	Instances int64  `json:"instances"`
	Hostname  string `json:"hostname,omitempty"`
	URL       string `json:"url,omitempty"`
	Message   string `json:"message,omitempty"`
	Age       string `json:"age"`
}

// HavenDatabase is a CloudNativePG Cluster summary.
type HavenDatabase struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Ready     bool   `json:"ready"`
	Instances int64  `json:"instances"`
	Status    string `json:"status,omitempty"`
	Age       string `json:"age"`
}

// HavenAdminBootstrap points at the Keycloak operator bootstrap secret (no password returned).
type HavenAdminBootstrap struct {
	Namespace  string `json:"namespace"`
	SecretName string `json:"secretName"`
	UsernameKey string `json:"usernameKey,omitempty"`
	PasswordKey string `json:"passwordKey,omitempty"`
	Exists     bool   `json:"exists"`
	Hint       string `json:"hint,omitempty"`
}

// HavenIdentitySummary is the Ops → Identity deck payload.
type HavenIdentitySummary struct {
	Planes     []IdentityPlane      `json:"planes"`
	Keycloaks  []HavenKeycloak      `json:"keycloaks"`
	Databases  []HavenDatabase      `json:"databases"`
	Admin      *HavenAdminBootstrap `json:"admin,omitempty"`
	OIDC       HavenOIDCHints       `json:"oidc"`
	Ready      bool                 `json:"ready"`
	Message    string               `json:"message,omitempty"`
}

type HavenOIDCHints struct {
	Issuer   string `json:"issuer,omitempty"`
	LoginURL string `json:"loginURL,omitempty"`
	ClientID string `json:"clientID,omitempty"`
	Configured bool `json:"configured"`
	Note     string `json:"note,omitempty"`
}
