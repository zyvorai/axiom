package platform

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Interactive PTY / node-shell sessions idle out when the browser stops
// answering ping or sending frames (same 10 m window as pod-proxy sessions).
const (
	terminalIdleTimeout = 10 * time.Minute
	terminalPingPeriod  = 30 * time.Second
)

// armTerminalIdle sets a read deadline, pong handler, and periodic ping so a
// vanished client closes the websocket (and cancels streamCtx) instead of
// leaving the SPDY exec open until the pod exits. Call after Upgrade; stop
// is cancelled by the caller when the session ends.
func armTerminalIdle(conn *websocket.Conn, writeMu *sync.Mutex, stop <-chan struct{}, onIdle func()) {
	reset := func() {
		_ = conn.SetReadDeadline(time.Now().Add(terminalIdleTimeout))
	}
	reset()
	conn.SetPongHandler(func(string) error {
		reset()
		return nil
	})
	go func() {
		t := time.NewTicker(terminalPingPeriod)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				writeMu.Lock()
				err := conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(5*time.Second))
				writeMu.Unlock()
				if err != nil {
					if onIdle != nil {
						onIdle()
					}
					return
				}
			}
		}
	}()
}

// touchTerminalIdle extends the read deadline after a client frame (resize or stdin).
func touchTerminalIdle(conn *websocket.Conn) {
	_ = conn.SetReadDeadline(time.Now().Add(terminalIdleTimeout))
}
