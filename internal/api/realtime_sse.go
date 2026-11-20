package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// StateEvent is pushed to SSE subscribers when cluster state changes.
type StateEvent struct {
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Action   string `json:"action"`
}

var eventHub = newSSEHub()

type sseHub struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func newSSEHub() *sseHub {
	return &sseHub{subs: make(map[chan []byte]struct{})}
}

func (h *sseHub) subscribe() chan []byte {
	ch := make(chan []byte, 16)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *sseHub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
	h.mu.Unlock()
}

func (h *sseHub) emit(ev StateEvent) {
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- data:
		default:
		}
	}
}

// EmitStateChanged notifies connected dashboards to refresh.
func EmitStateChanged(resource, action string) {
	eventHub.emit(StateEvent{Type: "stateChanged", Resource: resource, Action: action})
}

// RegisterSSE mounts the live event stream endpoint.
func RegisterSSE(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/events/stream", streamEvents)
}

func streamEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := eventHub.subscribe()
	defer eventHub.unsubscribe(ch)

	_, _ = fmt.Fprintf(w, "data: {\"type\":\"connected\"}\n\n")
	flusher.Flush()

	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case data, ok := <-ch:
			if !ok {
				return
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-ticker.C:
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}
