package api

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

type hijackable struct {
	http.ResponseWriter
	hijacked bool
}

func (h *hijackable) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.hijacked = true
	c1, c2 := net.Pipe()
	_ = c2.Close()
	return c1, bufio.NewReadWriter(bufio.NewReader(c1), bufio.NewWriter(c1)), nil
}

func (h *hijackable) Flush() {}

func TestStatusRecorderForwardsHijacker(t *testing.T) {
	inner := &hijackable{ResponseWriter: httptest.NewRecorder()}
	rec := &statusRecorder{ResponseWriter: inner, status: http.StatusOK}

	h, ok := any(rec).(http.Hijacker)
	if !ok {
		t.Fatal("statusRecorder must implement http.Hijacker for WebSocket upgrades")
	}
	conn, _, err := h.Hijack()
	if err != nil {
		t.Fatalf("Hijack: %v", err)
	}
	defer conn.Close()
	if !inner.hijacked {
		t.Fatal("expected underlying Hijack to be called")
	}
	if _, ok := any(rec).(http.Flusher); !ok {
		t.Fatal("statusRecorder must implement http.Flusher for log/SSE streams")
	}
}
