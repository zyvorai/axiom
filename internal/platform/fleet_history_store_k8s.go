//go:build kubernetes

package platform

import (
	"context"
	"math/rand/v2"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// fleetHistoryUpdateAttempts bounds the retries of a save on a write
// conflict (another replica saved its own key in between).
const fleetHistoryUpdateAttempts = 8

// configMapFleetHistoryBlobs keeps the snapshots in the ConfigMap
// zyvor-axiom-fleet-history, one key per replica.
type configMapFleetHistoryBlobs struct {
	client kubernetes.Interface
	ns     string
}

// NewK8sFleetHistoryStore is the ConfigMap store for replica in namespace ns
// ("" = POD_NAMESPACE, else axiom-system).
func NewK8sFleetHistoryStore(client kubernetes.Interface, ns, replica string) FleetHistoryStore {
	ns = strings.TrimSpace(ns)
	if ns == "" {
		ns = strings.TrimSpace(os.Getenv("POD_NAMESPACE"))
	}
	if ns == "" {
		ns = "axiom-system"
	}
	return newKeyedFleetHistoryStore(&configMapFleetHistoryBlobs{client: client, ns: ns}, replica, true)
}

// OpenFleetHistoryStore returns the ConfigMap store in kubernetes mode and an
// in-memory (not durable) store otherwise.
func OpenFleetHistoryStore(mode string, provider Provider) FleetHistoryStore {
	if strings.ToLower(mode) != "kubernetes" {
		return NewMemoryFleetHistoryStore(FleetHistoryReplicaID())
	}
	k, ok := CurrentKubernetesProvider(context.Background(), provider)
	if !ok || k == nil || k.client == nil {
		return NewMemoryFleetHistoryStore(FleetHistoryReplicaID())
	}
	return NewK8sFleetHistoryStore(k.client, "", FleetHistoryReplicaID())
}

func (b *configMapFleetHistoryBlobs) read(ctx context.Context) (map[string]string, error) {
	cm, err := b.client.CoreV1().ConfigMaps(b.ns).Get(ctx, FleetHistoryConfigMapName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(cm.Data))
	for k, v := range cm.Data {
		out[k] = v
	}
	return out, nil
}

// update is a read-modify-write guarded by the resourceVersion: a replica
// that saved its own key in between makes it retry against the new copy, so
// no replica's key is ever lost.
func (b *configMapFleetHistoryBlobs) update(ctx context.Context, fn func(map[string]string) error) error {
	cms := b.client.CoreV1().ConfigMaps(b.ns)
	var err error
	for attempt := 0; attempt < fleetHistoryUpdateAttempts; attempt++ {
		if attempt > 0 {
			step := time.Duration(attempt) * 20 * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(step + time.Duration(rand.Int64N(int64(step)))):
			}
		}
		var cm *corev1.ConfigMap
		cm, err = cms.Get(ctx, FleetHistoryConfigMapName, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		if err != nil { // not found: create it
			data := map[string]string{}
			if err = fn(data); err != nil {
				return err
			}
			_, err = cms.Create(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: FleetHistoryConfigMapName, Namespace: b.ns, Labels: map[string]string{"app": "zyvor-axiom"}},
				Data:       data,
			}, metav1.CreateOptions{})
		} else {
			cm = cm.DeepCopy()
			data := make(map[string]string, len(cm.Data)+1)
			for k, v := range cm.Data {
				data[k] = v
			}
			if err = fn(data); err != nil {
				return err
			}
			cm.Data = data
			_, err = cms.Update(ctx, cm, metav1.UpdateOptions{})
		}
		if err == nil || !(apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err)) {
			return err
		}
	}
	return err
}
