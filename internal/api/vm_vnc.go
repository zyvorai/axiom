package api

import (
	"net/http"

	"github.com/zyvorai/axiom/internal/platform"
)

func (s *Server) vmVNC(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	var p platform.Provider = s.provider
	if sp := s.switchable(); sp != nil {
		if cur := sp.CurrentProvider(r.Context()); cur != nil {
			p = cur
		}
	}
	if proxy, ok := p.(interface {
		VMVNCProxy(http.ResponseWriter, *http.Request, string, string) error
	}); ok {
		_ = proxy.VMVNCProxy(w, r, ns, name)
		return
	}
	fail(w, http.StatusNotImplemented, errSSO("in-browser VNC proxy requires the kubernetes build against a KubeVirt cluster"))
}
