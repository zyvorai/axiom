//go:build kubernetes

package platform

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const auditConfigMapName = "zyvor-axiom-audit"

// K8sAuditStore persists the audit ring as a JSON array in a ConfigMap,
// mirroring K8sIntegrationStore/K8sUserStore's pattern. 200 small entries
// comfortably fits well within the ConfigMap size limit.
type K8sAuditStore struct {
	client kubernetes.Interface
	ns     string
}

func NewK8sAuditStore(client kubernetes.Interface, ns string) *K8sAuditStore {
	ns = strings.TrimSpace(ns)
	if ns == "" {
		ns = strings.TrimSpace(os.Getenv("POD_NAMESPACE"))
	}
	if ns == "" {
		ns = "axiom-system"
	}
	return &K8sAuditStore{client: client, ns: ns}
}

func OpenAuditStore(mode string, provider Provider) AuditStore {
	if strings.ToLower(mode) != "kubernetes" {
		return nil
	}
	k, ok := CurrentKubernetesProvider(context.Background(), provider)
	if !ok || k == nil || k.client == nil {
		return nil
	}
	return NewK8sAuditStore(k.client, "")
}

func (s *K8sAuditStore) Load(ctx context.Context) ([]AuditEntry, error) {
	cm, err := s.client.CoreV1().ConfigMaps(s.ns).Get(ctx, auditConfigMapName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []AuditEntry
	if raw, ok := cm.Data["audit.json"]; ok {
		_ = json.Unmarshal([]byte(raw), &entries)
	}
	return entries, nil
}

// maxAuditPersistBytes keeps the persisted ring safely under the 1 MiB
// ConfigMap limit (the default 5000-entry ring is ~1+ MiB and would make every
// Save fail, silently ending persistence).
const maxAuditPersistBytes = 900 * 1024

// fitAuditEntries marshals entries and, if the result exceeds maxBytes, drops
// the OLDEST entries (the ring is oldest-first) until it fits.
func fitAuditEntries(entries []AuditEntry, maxBytes int) ([]byte, []AuditEntry, error) {
	for {
		raw, err := json.Marshal(entries)
		if err != nil {
			return nil, nil, err
		}
		if len(raw) <= maxBytes || len(entries) == 0 {
			return raw, entries, nil
		}
		// Drop in proportion to the overshoot (at least one entry) rather than
		// re-marshalling once per entry.
		drop := len(entries) - int(float64(len(entries))*float64(maxBytes)/float64(len(raw))*0.95)
		if drop < 1 {
			drop = 1
		}
		if drop > len(entries) {
			drop = len(entries)
		}
		entries = entries[drop:]
	}
}

func (s *K8sAuditStore) Save(ctx context.Context, entries []AuditEntry) error {
	raw, _, err := fitAuditEntries(entries, maxAuditPersistBytes)
	if err != nil {
		return err
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: auditConfigMapName, Namespace: s.ns, Labels: map[string]string{"app": "zyvor-axiom"}},
		Data:       map[string]string{"audit.json": string(raw)},
	}
	if _, err := s.client.CoreV1().ConfigMaps(s.ns).Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			_, err = s.client.CoreV1().ConfigMaps(s.ns).Create(ctx, cm, metav1.CreateOptions{})
			return err
		}
		return err
	}
	return nil
}
