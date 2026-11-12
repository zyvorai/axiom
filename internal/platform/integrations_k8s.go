//go:build kubernetes

package platform

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	integrationsConfigMap = "zyvor-axiom-integrations"
	integrationsSecret    = "zyvor-axiom-integrations"
)

type K8sIntegrationStore struct {
	client kubernetes.Interface
	ns     string
}

func NewK8sIntegrationStore(client kubernetes.Interface, ns string) *K8sIntegrationStore {
	ns = strings.TrimSpace(ns)
	if ns == "" {
		ns = strings.TrimSpace(os.Getenv("POD_NAMESPACE"))
	}
	if ns == "" {
		ns = "axiom-system"
	}
	return &K8sIntegrationStore{client: client, ns: ns}
}

func OpenIntegrationStore(mode string, provider Provider) IntegrationStore {
	if strings.ToLower(mode) != "kubernetes" {
		return NewMemoryIntegrationStore()
	}
	k, ok := CurrentKubernetesProvider(context.Background(), provider)
	if !ok || k == nil || k.client == nil {
		return NewMemoryIntegrationStore()
	}
	return NewK8sIntegrationStore(k.client, "")
}

func integrationsFromCM(cm *corev1.ConfigMap) Integrations {
	d := cm.Data
	port, _ := strconv.Atoi(d["postgresPort"])
	return Integrations{
		AtlasURL:        d["atlasUrl"],
		AtlasConsoleURL: d["atlasConsoleUrl"],
		OIDCIssuer:      d["oidcIssuer"],
		OIDCLoginURL:    d["oidcLoginUrl"],
		OIDCClientID:    d["oidcClientId"],
		PostgresHost:     d["postgresHost"],
		PostgresPort:     port,
		PostgresDatabase: d["postgresDatabase"],
		PostgresUser:     d["postgresUser"],
		PostgresSSLMode:  d["postgresSslMode"],
		ProbePodNamespace: d["probePodNamespace"],
		ProbePodName:      d["probePodName"],
		ProbePodContainer: d["probePodContainer"],
		ProbeURL:          d["probeUrl"],
		NotifyOnWarnings:  d["notifyOnWarnings"] == "true",
		AIEnabled:         d["aiEnabled"] == "true",
		AIProvider:        d["aiProvider"],
		AIBaseURL:         d["aiBaseUrl"],
		AIModel:           d["aiModel"],
		AIRedactOff:       d["aiRedact"] == "false", // absent => redaction stays on
		AgentAutoRuns:     d["agentAutoRuns"] == "true",
		BillingProvider:            d["billingProvider"],
		BillingTagKey:              d["billingTagKey"],
		BillingTagValue:            d["billingTagValue"],
		BillingAWSRegion:           d["billingAwsRegion"],
		BillingAzureTenantID:       d["billingAzureTenantId"],
		BillingAzureClientID:       d["billingAzureClientId"],
		BillingAzureSubscriptionID: d["billingAzureSubscriptionId"],
		BillingGCPProjectID:        d["billingGcpProjectId"],
		BillingGCPTable:            d["billingGcpTable"],
	}
}

func integrationsToCMData(in Integrations) map[string]string {
	return map[string]string{
		"atlasUrl":           in.AtlasURL,
		"atlasConsoleUrl":    in.AtlasConsoleURL,
		"oidcIssuer":         in.OIDCIssuer,
		"oidcLoginUrl":       in.OIDCLoginURL,
		"oidcClientId":       in.OIDCClientID,
		"postgresHost":       in.PostgresHost,
		"postgresPort":       strconv.Itoa(in.PostgresPort),
		"postgresDatabase":   in.PostgresDatabase,
		"postgresUser":       in.PostgresUser,
		"postgresSslMode":    in.PostgresSSLMode,
		"probePodNamespace":  in.ProbePodNamespace,
		"probePodName":       in.ProbePodName,
		"probePodContainer":  in.ProbePodContainer,
		"probeUrl":           in.ProbeURL,
		"notifyOnWarnings":   strconv.FormatBool(in.NotifyOnWarnings),
		"aiEnabled":          strconv.FormatBool(in.AIEnabled),
		"aiProvider":         in.AIProvider,
		"aiBaseUrl":          in.AIBaseURL,
		"aiModel":            in.AIModel,
		"aiRedact":           strconv.FormatBool(!in.AIRedactOff),
		"agentAutoRuns":      strconv.FormatBool(in.AgentAutoRuns),
		"billingProvider":            in.BillingProvider,
		"billingTagKey":              in.BillingTagKey,
		"billingTagValue":            in.BillingTagValue,
		"billingAwsRegion":           in.BillingAWSRegion,
		"billingAzureTenantId":       in.BillingAzureTenantID,
		"billingAzureClientId":       in.BillingAzureClientID,
		"billingAzureSubscriptionId": in.BillingAzureSubscriptionID,
		"billingGcpProjectId":        in.BillingGCPProjectID,
		"billingGcpTable":            in.BillingGCPTable,
	}
}

func (s *K8sIntegrationStore) Load(ctx context.Context) (Integrations, bool, error) {
	cm, err := s.client.CoreV1().ConfigMaps(s.ns).Get(ctx, integrationsConfigMap, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return Integrations{}, false, nil
	}
	if err != nil {
		return Integrations{}, false, err
	}
	out := integrationsFromCM(cm)
	sec, err := s.client.CoreV1().Secrets(s.ns).Get(ctx, integrationsSecret, metav1.GetOptions{})
	if err == nil {
		out.AtlasToken = string(sec.Data["atlasToken"])
		out.PostgresPassword = string(sec.Data["postgresPassword"])
		out.SlackWebhookURL = string(sec.Data["slackWebhookUrl"]) // webhook URLs are credentials: Secret, not ConfigMap
		out.AlertWebhookURL = string(sec.Data["alertWebhookUrl"])
		out.AIAPIKey = string(sec.Data["aiApiKey"]) // LLM API key: Secret only, never the ConfigMap
		// Cloud billing credentials: Secret only, never the ConfigMap.
		out.BillingAWSAccessKeyID = string(sec.Data["billingAwsAccessKeyId"])
		out.BillingAWSSecretAccessKey = string(sec.Data["billingAwsSecretAccessKey"])
		out.BillingAzureClientSecret = string(sec.Data["billingAzureClientSecret"])
		out.BillingGCPServiceAccountKey = string(sec.Data["billingGcpServiceAccountKey"])
	} else if !apierrors.IsNotFound(err) {
		return Integrations{}, false, err
	}
	return out.Normalize(), true, nil
}

func (s *K8sIntegrationStore) Save(ctx context.Context, in Integrations) error {
	in = in.Normalize()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      integrationsConfigMap,
			Namespace: s.ns,
			Labels:    map[string]string{"app": "zyvor-axiom"},
		},
		Data: integrationsToCMData(in),
	}
	if _, err := s.client.CoreV1().ConfigMaps(s.ns).Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			if _, err := s.client.CoreV1().ConfigMaps(s.ns).Create(ctx, cm, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("save integrations configmap: %w", err)
			}
		} else {
			return fmt.Errorf("save integrations configmap: %w", err)
		}
	}
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      integrationsSecret,
			Namespace: s.ns,
			Labels:    map[string]string{"app": "zyvor-axiom"},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"atlasToken":       []byte(in.AtlasToken),
			"postgresPassword": []byte(in.PostgresPassword),
			"slackWebhookUrl":  []byte(in.SlackWebhookURL),
			"alertWebhookUrl":  []byte(in.AlertWebhookURL),
			"aiApiKey":         []byte(in.AIAPIKey),

			"billingAwsAccessKeyId":       []byte(in.BillingAWSAccessKeyID),
			"billingAwsSecretAccessKey":   []byte(in.BillingAWSSecretAccessKey),
			"billingAzureClientSecret":    []byte(in.BillingAzureClientSecret),
			"billingGcpServiceAccountKey": []byte(in.BillingGCPServiceAccountKey),
		},
	}
	if _, err := s.client.CoreV1().Secrets(s.ns).Update(ctx, sec, metav1.UpdateOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			if _, err := s.client.CoreV1().Secrets(s.ns).Create(ctx, sec, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("save integrations secret: %w", err)
			}
		} else {
			return fmt.Errorf("save integrations secret: %w", err)
		}
	}
	return nil
}
