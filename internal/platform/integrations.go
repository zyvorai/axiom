package platform

import (
	"context"
	"strings"
	"sync"
)

// Integrations holds runtime product wiring editable from the Axiom Settings UX.
// Env vars seed defaults on first boot; UX overrides persist via IntegrationStore.
type Integrations struct {
	AtlasURL        string `json:"atlasUrl"`
	AtlasToken      string `json:"atlasToken,omitempty"`
	AtlasConsoleURL string `json:"atlasConsoleUrl"`
	OIDCIssuer      string `json:"oidcIssuer"`
	OIDCLoginURL    string `json:"oidcLoginUrl"`
	OIDCClientID    string `json:"oidcClientId"`

	PostgresHost     string `json:"postgresHost"`
	PostgresPort     int    `json:"postgresPort"`
	PostgresDatabase string `json:"postgresDatabase"`
	PostgresUser     string `json:"postgresUser"`
	PostgresPassword string `json:"postgresPassword,omitempty"`
	PostgresSSLMode  string `json:"postgresSslMode"`

	ProbePodNamespace  string `json:"probePodNamespace"`
	ProbePodName       string `json:"probePodName"`
	ProbePodContainer  string `json:"probePodContainer"`
	ProbeURL           string `json:"probeUrl"`

	SlackWebhookURL  string `json:"slackWebhookUrl,omitempty"`
	AlertWebhookURL  string `json:"alertWebhookUrl,omitempty"`
	NotifyOnWarnings bool   `json:"notifyOnWarnings"`

	// AI assistant (see ai.go). Off by default. The API key lives in the
	// integrations Secret only. AIRedactOff is inverted so the zero value is the
	// safe default (redaction on).
	AIEnabled   bool   `json:"aiEnabled"`
	AIProvider  string `json:"aiProvider"`
	AIBaseURL   string `json:"aiBaseUrl"`
	AIModel     string `json:"aiModel"`
	AIAPIKey    string `json:"aiApiKey,omitempty"`
	AIRedactOff bool   `json:"aiRedactOff"`

	// Actual-bill reconciliation (EXPERIMENTAL, see billing.go). Credentials
	// (AWS access key pair, Azure client secret, GCP service-account key) live
	// in the integrations Secret only and are never returned.
	BillingProvider            string `json:"billingProvider"`
	BillingTagKey              string `json:"billingTagKey"`
	BillingTagValue            string `json:"billingTagValue"`
	BillingAWSRegion           string `json:"billingAwsRegion"`
	BillingAWSAccessKeyID      string `json:"billingAwsAccessKeyId,omitempty"`
	BillingAWSSecretAccessKey  string `json:"billingAwsSecretAccessKey,omitempty"`
	BillingAzureTenantID       string `json:"billingAzureTenantId"`
	BillingAzureClientID       string `json:"billingAzureClientId"`
	BillingAzureSubscriptionID string `json:"billingAzureSubscriptionId"`
	BillingAzureClientSecret   string `json:"billingAzureClientSecret,omitempty"`
	BillingGCPProjectID        string `json:"billingGcpProjectId"`
	BillingGCPTable            string `json:"billingGcpTable"`
	BillingGCPServiceAccountKey string `json:"billingGcpServiceAccountKey,omitempty"`
}

// IntegrationsView is the safe GET payload (secrets never returned in cleartext).
type IntegrationsView struct {
	AtlasURL         string `json:"atlasUrl"`
	AtlasTokenSet    bool   `json:"atlasTokenSet"`
	AtlasConsoleURL  string `json:"atlasConsoleUrl"`
	OIDCIssuer       string `json:"oidcIssuer"`
	OIDCLoginURL     string `json:"oidcLoginUrl"`
	OIDCClientID     string `json:"oidcClientId"`
	AtlasConfigured  bool   `json:"atlasConfigured"`
	OIDCConfigured   bool   `json:"oidcConfigured"`

	PostgresHost        string `json:"postgresHost"`
	PostgresPort        int    `json:"postgresPort"`
	PostgresDatabase    string `json:"postgresDatabase"`
	PostgresUser        string `json:"postgresUser"`
	PostgresPasswordSet bool   `json:"postgresPasswordSet"`
	PostgresSSLMode     string `json:"postgresSslMode"`
	PostgresConfigured  bool   `json:"postgresConfigured"`

	ProbePodNamespace string `json:"probePodNamespace"`
	ProbePodName      string `json:"probePodName"`
	ProbePodContainer string `json:"probePodContainer"`
	ProbeURL          string `json:"probeUrl"`
	ProbeConfigured   bool   `json:"probeConfigured"`

	SlackWebhookURLSet bool `json:"slackWebhookUrlSet"`
	AlertWebhookURLSet bool `json:"alertWebhookUrlSet"`
	NotifyOnWarnings   bool `json:"notifyOnWarnings"`
	NotifyConfigured   bool `json:"notifyConfigured"`

	AIEnabled    bool   `json:"aiEnabled"`
	AIProvider   string `json:"aiProvider"`
	AIBaseURL    string `json:"aiBaseUrl"`
	AIModel      string `json:"aiModel"`
	AIAPIKeySet  bool   `json:"aiApiKeySet"`
	AIRedact     bool   `json:"aiRedact"`
	AIConfigured bool   `json:"aiConfigured"`

	BillingProvider                string `json:"billingProvider"`
	BillingTagKey                  string `json:"billingTagKey"`
	BillingTagValue                string `json:"billingTagValue"`
	BillingAWSRegion               string `json:"billingAwsRegion"`
	BillingAWSAccessKeyIDSet       bool   `json:"billingAwsAccessKeyIdSet"`
	BillingAWSSecretAccessKeySet   bool   `json:"billingAwsSecretAccessKeySet"`
	BillingAzureTenantID           string `json:"billingAzureTenantId"`
	BillingAzureClientID           string `json:"billingAzureClientId"`
	BillingAzureSubscriptionID     string `json:"billingAzureSubscriptionId"`
	BillingAzureClientSecretSet    bool   `json:"billingAzureClientSecretSet"`
	BillingGCPProjectID            string `json:"billingGcpProjectId"`
	BillingGCPTable                string `json:"billingGcpTable"`
	BillingGCPServiceAccountKeySet bool   `json:"billingGcpServiceAccountKeySet"`
	BillingConfigured              bool   `json:"billingConfigured"`
}

func (i Integrations) View() IntegrationsView {
	return i.billingView(IntegrationsView{
		AtlasURL:        i.AtlasURL,
		AtlasTokenSet:   strings.TrimSpace(i.AtlasToken) != "",
		AtlasConsoleURL: i.AtlasConsoleURL,
		OIDCIssuer:      i.OIDCIssuer,
		OIDCLoginURL:    i.OIDCLoginURL,
		OIDCClientID:    i.OIDCClientID,
		AtlasConfigured: strings.TrimSpace(i.AtlasURL) != "",
		OIDCConfigured:  strings.TrimSpace(i.OIDCLoginURL) != "" || strings.TrimSpace(i.OIDCIssuer) != "",

		PostgresHost:        i.PostgresHost,
		PostgresPort:        i.PostgresPort,
		PostgresDatabase:    i.PostgresDatabase,
		PostgresUser:        i.PostgresUser,
		PostgresPasswordSet: strings.TrimSpace(i.PostgresPassword) != "",
		PostgresSSLMode:     i.PostgresSSLMode,
		PostgresConfigured:  strings.TrimSpace(i.PostgresHost) != "",

		ProbePodNamespace: i.ProbePodNamespace,
		ProbePodName:      i.ProbePodName,
		ProbePodContainer: i.ProbePodContainer,
		ProbeURL:          i.ProbeURL,
		ProbeConfigured:   strings.TrimSpace(i.ProbeURL) != "",

		SlackWebhookURLSet: strings.TrimSpace(i.SlackWebhookURL) != "",
		AlertWebhookURLSet: strings.TrimSpace(i.AlertWebhookURL) != "",
		NotifyOnWarnings:   i.NotifyOnWarnings,
		NotifyConfigured:   strings.TrimSpace(i.SlackWebhookURL) != "" || strings.TrimSpace(i.AlertWebhookURL) != "",

		AIEnabled:    i.AIEnabled,
		AIProvider:   i.aiProviderOrDefault(),
		AIBaseURL:    i.AIBaseURL,
		AIModel:      i.AIModel,
		AIAPIKeySet:  strings.TrimSpace(i.AIAPIKey) != "",
		AIRedact:     !i.AIRedactOff,
		AIConfigured: i.aiConfigured(),
	})
}

func (i Integrations) Normalize() Integrations {
	i.AtlasURL = strings.TrimRight(strings.TrimSpace(i.AtlasURL), "/")
	i.AtlasToken = strings.TrimSpace(i.AtlasToken)
	i.AtlasConsoleURL = strings.TrimRight(strings.TrimSpace(i.AtlasConsoleURL), "/")
	i.OIDCIssuer = strings.TrimRight(strings.TrimSpace(i.OIDCIssuer), "/")
	i.OIDCLoginURL = strings.TrimSpace(i.OIDCLoginURL)
	i.OIDCClientID = strings.TrimSpace(i.OIDCClientID)

	i.PostgresHost = strings.TrimSpace(i.PostgresHost)
	if i.PostgresPort <= 0 {
		i.PostgresPort = 5432
	}
	i.PostgresDatabase = strings.TrimSpace(i.PostgresDatabase)
	i.PostgresUser = strings.TrimSpace(i.PostgresUser)
	i.PostgresPassword = strings.TrimSpace(i.PostgresPassword)
	i.PostgresSSLMode = strings.TrimSpace(i.PostgresSSLMode)
	if i.PostgresSSLMode == "" {
		i.PostgresSSLMode = "disable"
	}

	i.ProbePodNamespace = strings.TrimSpace(i.ProbePodNamespace)
	i.ProbePodName = strings.TrimSpace(i.ProbePodName)
	i.ProbePodContainer = strings.TrimSpace(i.ProbePodContainer)
	i.ProbeURL = strings.TrimSpace(i.ProbeURL)

	i.SlackWebhookURL = strings.TrimSpace(i.SlackWebhookURL)
	i.AlertWebhookURL = strings.TrimSpace(i.AlertWebhookURL)
	i = i.normalizeAI()
	i = i.normalizeBilling()
	return i
}

type IntegrationsPatch struct {
	AtlasURL        *string `json:"atlasUrl"`
	AtlasToken      *string `json:"atlasToken"`
	AtlasTokenClear bool    `json:"atlasTokenClear"`
	AtlasConsoleURL *string `json:"atlasConsoleUrl"`
	OIDCIssuer      *string `json:"oidcIssuer"`
	OIDCLoginURL    *string `json:"oidcLoginUrl"`
	OIDCClientID    *string `json:"oidcClientId"`

	PostgresHost        *string `json:"postgresHost"`
	PostgresPort        *int    `json:"postgresPort"`
	PostgresDatabase    *string `json:"postgresDatabase"`
	PostgresUser        *string `json:"postgresUser"`
	PostgresPassword    *string `json:"postgresPassword"`
	PostgresPasswordClear bool  `json:"postgresPasswordClear"`
	PostgresSSLMode     *string `json:"postgresSslMode"`

	ProbePodNamespace *string `json:"probePodNamespace"`
	ProbePodName      *string `json:"probePodName"`
	ProbePodContainer *string `json:"probePodContainer"`
	ProbeURL          *string `json:"probeUrl"`

	SlackWebhookURL      *string `json:"slackWebhookUrl"`
	SlackWebhookURLClear bool    `json:"slackWebhookUrlClear"`
	AlertWebhookURL      *string `json:"alertWebhookUrl"`
	AlertWebhookURLClear bool    `json:"alertWebhookUrlClear"`
	NotifyOnWarnings     *bool   `json:"notifyOnWarnings"`

	AIEnabled     *bool   `json:"aiEnabled"`
	AIProvider    *string `json:"aiProvider"`
	AIBaseURL     *string `json:"aiBaseUrl"`
	AIModel       *string `json:"aiModel"`
	AIAPIKey      *string `json:"aiApiKey"`
	AIAPIKeyClear bool    `json:"aiApiKeyClear"`
	AIRedact      *bool   `json:"aiRedact"`

	BillingProvider                  *string `json:"billingProvider"`
	BillingTagKey                    *string `json:"billingTagKey"`
	BillingTagValue                  *string `json:"billingTagValue"`
	BillingAWSRegion                 *string `json:"billingAwsRegion"`
	BillingAWSAccessKeyID            *string `json:"billingAwsAccessKeyId"`
	BillingAWSAccessKeyIDClear       bool    `json:"billingAwsAccessKeyIdClear"`
	BillingAWSSecretAccessKey        *string `json:"billingAwsSecretAccessKey"`
	BillingAWSSecretAccessKeyClear   bool    `json:"billingAwsSecretAccessKeyClear"`
	BillingAzureTenantID             *string `json:"billingAzureTenantId"`
	BillingAzureClientID             *string `json:"billingAzureClientId"`
	BillingAzureSubscriptionID       *string `json:"billingAzureSubscriptionId"`
	BillingAzureClientSecret         *string `json:"billingAzureClientSecret"`
	BillingAzureClientSecretClear    bool    `json:"billingAzureClientSecretClear"`
	BillingGCPProjectID              *string `json:"billingGcpProjectId"`
	BillingGCPTable                  *string `json:"billingGcpTable"`
	BillingGCPServiceAccountKey      *string `json:"billingGcpServiceAccountKey"`
	BillingGCPServiceAccountKeyClear bool    `json:"billingGcpServiceAccountKeyClear"`
}

func (i Integrations) Apply(p IntegrationsPatch) Integrations {
	if p.AtlasURL != nil {
		i.AtlasURL = *p.AtlasURL
	}
	if p.AtlasTokenClear {
		i.AtlasToken = ""
	} else if p.AtlasToken != nil && strings.TrimSpace(*p.AtlasToken) != "" {
		i.AtlasToken = *p.AtlasToken
	}
	if p.AtlasConsoleURL != nil {
		i.AtlasConsoleURL = *p.AtlasConsoleURL
	}
	if p.OIDCIssuer != nil {
		i.OIDCIssuer = *p.OIDCIssuer
	}
	if p.OIDCLoginURL != nil {
		i.OIDCLoginURL = *p.OIDCLoginURL
	}
	if p.OIDCClientID != nil {
		i.OIDCClientID = *p.OIDCClientID
	}

	if p.PostgresHost != nil {
		i.PostgresHost = *p.PostgresHost
	}
	if p.PostgresPort != nil && *p.PostgresPort > 0 {
		i.PostgresPort = *p.PostgresPort
	}
	if p.PostgresDatabase != nil {
		i.PostgresDatabase = *p.PostgresDatabase
	}
	if p.PostgresUser != nil {
		i.PostgresUser = *p.PostgresUser
	}
	if p.PostgresPasswordClear {
		i.PostgresPassword = ""
	} else if p.PostgresPassword != nil && strings.TrimSpace(*p.PostgresPassword) != "" {
		i.PostgresPassword = *p.PostgresPassword
	}
	if p.PostgresSSLMode != nil {
		i.PostgresSSLMode = *p.PostgresSSLMode
	}

	if p.ProbePodNamespace != nil {
		i.ProbePodNamespace = *p.ProbePodNamespace
	}
	if p.ProbePodName != nil {
		i.ProbePodName = *p.ProbePodName
	}
	if p.ProbePodContainer != nil {
		i.ProbePodContainer = *p.ProbePodContainer
	}
	if p.ProbeURL != nil {
		i.ProbeURL = *p.ProbeURL
	}

	if p.SlackWebhookURLClear {
		i.SlackWebhookURL = ""
	} else if p.SlackWebhookURL != nil && strings.TrimSpace(*p.SlackWebhookURL) != "" {
		i.SlackWebhookURL = *p.SlackWebhookURL
	}
	if p.AlertWebhookURLClear {
		i.AlertWebhookURL = ""
	} else if p.AlertWebhookURL != nil && strings.TrimSpace(*p.AlertWebhookURL) != "" {
		i.AlertWebhookURL = *p.AlertWebhookURL
	}
	if p.NotifyOnWarnings != nil {
		i.NotifyOnWarnings = *p.NotifyOnWarnings
	}
	i = i.applyAIPatch(p)
	i = i.applyBillingPatch(p)
	return i.Normalize()
}

// IntegrationStore persists UX overrides across restarts.
type IntegrationStore interface {
	Load(ctx context.Context) (Integrations, bool, error)
	Save(ctx context.Context, in Integrations) error
}

type MemoryIntegrationStore struct {
	mu   sync.Mutex
	have bool
	data Integrations
}

func NewMemoryIntegrationStore() *MemoryIntegrationStore {
	return &MemoryIntegrationStore{}
}

func (m *MemoryIntegrationStore) Load(context.Context) (Integrations, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.data, m.have, nil
}

func (m *MemoryIntegrationStore) Save(_ context.Context, in Integrations) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = in.Normalize()
	m.have = true
	return nil
}

// IntegrationHub owns live Atlas + OIDC wiring for the API server.
type IntegrationHub struct {
	mu      sync.RWMutex
	current Integrations
	atlas   *AtlasClient
	store   IntegrationStore
}

func NewIntegrationHub(defaults Integrations, store IntegrationStore) *IntegrationHub {
	if store == nil {
		store = NewMemoryIntegrationStore()
	}
	h := &IntegrationHub{current: defaults.Normalize(), store: store}
	h.rebuildAtlas()
	return h
}

func (h *IntegrationHub) Load(ctx context.Context) error {
	if h == nil || h.store == nil {
		return nil
	}
	saved, ok, err := h.store.Load(ctx)
	if err != nil || !ok {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.current = saved.Normalize()
	h.rebuildAtlas()
	return nil
}

func (h *IntegrationHub) Get() Integrations {
	if h == nil {
		return Integrations{}
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.current
}

func (h *IntegrationHub) View() IntegrationsView {
	return h.Get().View()
}

func (h *IntegrationHub) Atlas() *AtlasClient {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.atlas
}

func (h *IntegrationHub) Update(ctx context.Context, patch IntegrationsPatch) (IntegrationsView, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.current = h.current.Apply(patch)
	h.rebuildAtlas()
	if err := h.store.Save(ctx, h.current); err != nil {
		return h.current.View(), err
	}
	return h.current.View(), nil
}

func (h *IntegrationHub) rebuildAtlas() {
	h.atlas = NewAtlasClient(h.current.AtlasURL, h.current.AtlasToken)
}
