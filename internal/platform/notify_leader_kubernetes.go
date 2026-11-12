//go:build kubernetes

package platform

import (
	"context"
	"os"
	"strings"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
)

// K8sLeaseStore acquires named Leases in the Axiom namespace.
type K8sLeaseStore struct {
	client kubernetes.Interface
	ns     string
}

func NewK8sLeaseStore(client kubernetes.Interface, ns string) *K8sLeaseStore {
	ns = strings.TrimSpace(ns)
	if ns == "" {
		ns = strings.TrimSpace(os.Getenv("POD_NAMESPACE"))
	}
	if ns == "" {
		ns = "axiom-system"
	}
	return &K8sLeaseStore{client: client, ns: ns}
}

// OpenNotifyLeader returns a Lease-backed NotifyLeader in kubernetes mode, else nil (caller keeps always-lead).
func OpenNotifyLeader(mode string, provider Provider) NotifyLeader {
	if strings.ToLower(mode) != "kubernetes" {
		return nil
	}
	k, ok := CurrentKubernetesProvider(context.Background(), provider)
	if !ok || k == nil || k.client == nil {
		return nil
	}
	return NewLeaseNotifyLeader(NewK8sLeaseStore(k.client, ""), "", 15*time.Second)
}

func (s *K8sLeaseStore) TryAcquire(ctx context.Context, name, holder string, ttl time.Duration) (bool, error) {
	now := metav1.NewMicroTime(time.Now())
	lease, err := s.client.CoordinationV1().Leases(s.ns).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		obj := &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.ns, Labels: map[string]string{"app": "zyvor-axiom"}},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity:       ptr.To(holder),
				LeaseDurationSeconds: ptr.To(int32(ttl.Seconds())),
				AcquireTime:          &now,
				RenewTime:            &now,
			},
		}
		_, err = s.client.CoordinationV1().Leases(s.ns).Create(ctx, obj, metav1.CreateOptions{})
		if apierrors.IsAlreadyExists(err) {
			return s.TryAcquire(ctx, name, holder, ttl)
		}
		return err == nil, err
	}
	if err != nil {
		return false, err
	}
	expired := true
	if lease.Spec.RenewTime != nil && lease.Spec.LeaseDurationSeconds != nil {
		expired = time.Since(lease.Spec.RenewTime.Time) > time.Duration(*lease.Spec.LeaseDurationSeconds)*time.Second
	}
	mine := lease.Spec.HolderIdentity != nil && *lease.Spec.HolderIdentity == holder
	if !mine && !expired {
		return false, nil
	}
	lease.Spec.HolderIdentity = ptr.To(holder)
	lease.Spec.LeaseDurationSeconds = ptr.To(int32(ttl.Seconds()))
	lease.Spec.RenewTime = &now
	if lease.Spec.AcquireTime == nil || expired {
		lease.Spec.AcquireTime = &now
	}
	_, err = s.client.CoordinationV1().Leases(s.ns).Update(ctx, lease, metav1.UpdateOptions{})
	if apierrors.IsConflict(err) {
		return false, nil
	}
	return err == nil, err
}
