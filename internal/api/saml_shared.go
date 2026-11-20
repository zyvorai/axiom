package api

import (
	"log"

	"github.com/zyvorai/axiom/internal/auth"
)

// AttachSharedSAMLStore dual-writes outstanding AuthnRequest IDs to a shared
// RevocationStore so multi-replica SAML login→ACS works without sticky sessions.
func (s *Server) AttachSharedSAMLStore(store auth.RevocationStore) {
	if s == nil || store == nil {
		return
	}
	s.samlReqs.setShared(store)
	log.Printf("saml: shared AuthnRequest store enabled")
}

type samlSharedSnap struct {
	IDs map[string]int64 `json:"ids"`
}

func (st *samlRequestStore) setShared(store auth.RevocationStore) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.shared = store
}
