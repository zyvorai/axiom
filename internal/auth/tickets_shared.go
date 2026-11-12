package auth

import (
	"encoding/json"
	"log"
	"sync"
	"time"
)

// SharedTicketBacking dual-writes outstanding stream ticket digests so
// multi-replica WebSocket/SSE redeem works without sticky sessions.
type SharedTicketBacking struct {
	store RevocationStore
	mu    sync.Mutex
}

type ticketSharedSnap struct {
	Entries map[string]ticketSharedEntry `json:"entries"`
}

type ticketSharedEntry struct {
	User string `json:"user"`
	Role string `json:"role"`
	Path string `json:"path"`
	NS   string `json:"namespaces,omitempty"`
	Exp  int64  `json:"exp"`
}

// AttachSharedTickets enables ConfigMap-backed ticket redeem across replicas.
func (t *TicketStore) AttachShared(store RevocationStore) {
	if t == nil || store == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.shared = &SharedTicketBacking{store: store}
	log.Printf("auth: shared stream-ticket store enabled")
}

// shared field is on TicketStore — patched in tickets.go
func (b *SharedTicketBacking) put(digest string, e ticketEntry) {
	if b == nil || b.store == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	raw, _ := b.store.Load()
	var snap ticketSharedSnap
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &snap)
	}
	if snap.Entries == nil {
		snap.Entries = map[string]ticketSharedEntry{}
	}
	now := time.Now().Unix()
	for k, v := range snap.Entries {
		if v.Exp <= now {
			delete(snap.Entries, k)
		}
	}
	ns := ""
	if len(e.ident.Namespaces) > 0 {
		ns = e.ident.Namespaces[0]
		for i := 1; i < len(e.ident.Namespaces); i++ {
			ns += "," + e.ident.Namespaces[i]
		}
	}
	snap.Entries[digest] = ticketSharedEntry{
		User: e.ident.User, Role: e.ident.Role, Path: e.path, NS: ns, Exp: e.exp.Unix(),
	}
	out, _ := json.Marshal(snap)
	_ = b.store.Save(out)
}

func (b *SharedTicketBacking) take(digest string) (ticketEntry, bool) {
	if b == nil || b.store == nil {
		return ticketEntry{}, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	raw, err := b.store.Load()
	if err != nil || len(raw) == 0 {
		return ticketEntry{}, false
	}
	var snap ticketSharedSnap
	if json.Unmarshal(raw, &snap) != nil || snap.Entries == nil {
		return ticketEntry{}, false
	}
	se, ok := snap.Entries[digest]
	if !ok || se.Exp < time.Now().Unix() {
		return ticketEntry{}, false
	}
	delete(snap.Entries, digest)
	out, _ := json.Marshal(snap)
	_ = b.store.Save(out)
	var ns []string
	if se.NS != "" {
		ns = splitCSV(se.NS)
	}
	return ticketEntry{
		ident: Identity{User: se.User, Role: se.Role, Namespaces: ns},
		path:  se.Path,
		exp:   time.Unix(se.Exp, 0),
	}, true
}

func splitCSV(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	return out
}
