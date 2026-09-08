//go:build kubernetes

package platform

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
	"k8s.io/client-go/rest"
)

// VMVNCProxy upgrades the browser connection and tunnels binary frames to the
// KubeVirt VMI VNC subresource. Compatible with noVNC-style clients.
func (k *KubernetesProvider) VMVNCProxy(w http.ResponseWriter, r *http.Request, ns, name string) error {
	if k.config == nil {
		http.Error(w, "kubernetes config unavailable", http.StatusBadGateway)
		return fmt.Errorf("no rest config")
	}
	host := k.config.Host
	u, err := url.Parse(host)
	if err != nil || u.Host == "" {
		// Host may be bare host:port
		u = &url.URL{Host: strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")}
	}
	if rest.IsConfigTransportTLS(*k.config) {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = fmt.Sprintf("/apis/subresources.kubevirt.io/v1/namespaces/%s/virtualmachineinstances/%s/vnc", ns, name)
	u.RawQuery = ""

	tlsConfig, err := rest.TLSConfigFor(k.config)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return err
	}
	dialer := websocket.Dialer{
		TLSClientConfig: tlsConfig,
		Subprotocols:    []string{"binary.kubevirt.io", "base64.binary.kubevirt.io"},
	}
	hdr := http.Header{}
	if k.config.BearerToken != "" {
		hdr.Set("Authorization", "Bearer "+k.config.BearerToken)
	}
	backend, resp, err := dialer.Dial(u.String(), hdr)
	if err != nil {
		msg := err.Error()
		if resp != nil {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			resp.Body.Close()
			msg = fmt.Sprintf("%s: %s", msg, string(b))
		}
		http.Error(w, "kubevirt vnc dial failed: "+msg, http.StatusBadGateway)
		return err
	}
	defer backend.Close()

	client, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return err
	}
	defer client.Close()

	errc := make(chan error, 2)
	go func() { errc <- copyWS(backend, client) }()
	go func() { errc <- copyWS(client, backend) }()
	return <-errc
}

func copyWS(dst, src *websocket.Conn) error {
	for {
		typ, msg, err := src.ReadMessage()
		if err != nil {
			return err
		}
		if err := dst.WriteMessage(typ, msg); err != nil {
			return err
		}
	}
}
