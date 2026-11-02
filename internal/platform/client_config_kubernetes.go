//go:build kubernetes

package platform

import (
	"os"
	"strings"
	"sync/atomic"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	// apiRequestTimeout bounds ordinary (non-streaming) API calls.
	apiRequestTimeout = 60 * time.Second
	// client-go defaults to 5 QPS / burst 10, which throttles a single
	// Overview load (about nine list calls) and any page that fans out.
	apiQPS   = 50
	apiBurst = 100
)

// applyClientDefaults gives every provider construction path (in-cluster,
// multi-context kubeconfig, runtime-registered remote cluster) the same
// timeout and rate limits. Previously only one path set a timeout, so a hung
// remote API server blocked any call without its own deadline.
func applyClientDefaults(cfg *rest.Config) {
	cfg.UserAgent = "zyvor-axiom/0.1"
	if cfg.Timeout == 0 {
		cfg.Timeout = apiRequestTimeout
	}
	if cfg.QPS == 0 {
		cfg.QPS = apiQPS
	}
	if cfg.Burst == 0 {
		cfg.Burst = apiBurst
	}
}

// streamingClient returns a clientset with no overall request timeout, for
// long-lived streams (log follow). rest.Config.Timeout becomes
// http.Client.Timeout, which also covers reading the response body, so with a
// timeout set a followed log is cut off mid-stream. Falls back to the normal
// client when there is no config (unit tests with a fake clientset).
func (k *KubernetesProvider) streamingClient() kubernetes.Interface {
	if k.config == nil {
		return k.client
	}
	cp := rest.CopyConfig(k.config)
	cp.Timeout = 0
	c, err := kubernetes.NewForConfig(cp)
	if err != nil {
		return k.client
	}
	return c
}

// bearerTokenFor returns the current bearer token for cfg. In-cluster
// ServiceAccount tokens are bound and rotate (about hourly); client-go's own
// transport re-reads BearerTokenFile, but code that copies cfg.BearerToken into
// a hand-built request (the VM VNC proxy) would otherwise keep a frozen,
// eventually-expired value.
func bearerTokenFor(cfg *rest.Config) string {
	if cfg.BearerTokenFile != "" {
		if b, err := os.ReadFile(cfg.BearerTokenFile); err == nil {
			if t := strings.TrimSpace(string(b)); t != "" {
				return t
			}
		}
	}
	return cfg.BearerToken
}

// limitedBuffer collects at most max bytes and silently drops the rest, so a
// runaway one-shot exec (`yes`, `cat /dev/urandom | base64`) cannot grow
// Axiom's memory for the whole exec deadline.
type limitedBuffer struct {
	buf       []byte
	max       int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	room := b.max - len(b.buf)
	if room <= 0 {
		b.truncated = true
		return len(p), nil // swallow so the stream keeps draining instead of erroring
	}
	if len(p) > room {
		b.buf = append(b.buf, p[:room]...)
		b.truncated = true
		return len(p), nil
	}
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	if b.truncated {
		return string(b.buf) + "\n…[output truncated]"
	}
	return string(b.buf)
}

func (b *limitedBuffer) Len() int { return len(b.buf) }

const (
	maxExecOutputBytes = 1 << 20 // 1 MiB per one-shot exec
	maxExecWSInput     = 1 << 20 // largest single terminal input message
	maxExecWSSessions  = 64      // concurrent interactive terminals per Axiom process
)

var execWSSessions atomic.Int32

// acquireExecWSSession reserves a terminal slot; the caller must call the
// returned release func. ok is false when the cap is reached.
func acquireExecWSSession() (release func(), ok bool) {
	if execWSSessions.Add(1) > maxExecWSSessions {
		execWSSessions.Add(-1)
		return nil, false
	}
	return func() { execWSSessions.Add(-1) }, true
}
