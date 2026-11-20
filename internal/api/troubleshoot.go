package api

import (
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/zyvorai/axiom/internal/platform"
)

// registerTroubleshooting wires the Workstream B routes (timeline, cluster
// health check, workload logs, debug containers, node shell). Kept in its own
// function so the shared route block in server.go gains a single line.
func (s *Server) registerTroubleshooting(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/timeline", s.timeline)
	mux.HandleFunc("GET /api/v1/changes", s.changes)
	mux.HandleFunc("GET /api/v1/diagnose", s.clusterHealth)
	mux.HandleFunc("GET /api/v1/workloads/{namespace}/{kind}/{name}/logs", s.workloadLogs)
	mux.HandleFunc("POST /api/v1/pods/{namespace}/{name}/debug", s.debugPod)
	mux.HandleFunc("GET /api/v1/debug/capabilities", s.debugCapabilities)
	registerNodeShell(mux, s.provider)
}

func (s *Server) clusterHealth(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ClusterHealth(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

const (
	defaultWorkloadLogTail = 100
	maxWorkloadLogTail     = 5000
)

// workloadLogs streams the merged logs of every pod of a workload, each line
// prefixed "[pod/container] ". Timestamps are always present (RFC3339Nano after
// the prefix); the UI decides whether to show them.
func (s *Server) workloadLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	opts := platform.WorkloadLogOptions{
		Container: q.Get("container"),
		Follow:    q.Get("follow") == "true",
		Previous:  q.Get("previous") == "true",
		Tail:      defaultWorkloadLogTail,
	}
	if v := q.Get("tail"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			opts.Tail = min(n, maxWorkloadLogTail)
		}
	}
	stream, err := s.provider.WorkloadLogs(r.Context(), r.PathValue("namespace"), r.PathValue("kind"), r.PathValue("name"), opts)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	defer stream.Close()
	// Closing the stream on disconnect unblocks Read for followed logs.
	go func() {
		<-r.Context().Done()
		_ = stream.Close()
	}()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, err := stream.Read(buf)
		if n > 0 {
			_, _ = w.Write(buf[:n])
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) && r.Context().Err() == nil {
				log.Printf("workload log stream: %v", err)
			}
			return
		}
	}
}

// debugPod adds an ephemeral debug container to a pod (operator+; audited).
func (s *Server) debugPod(w http.ResponseWriter, r *http.Request) {
	var req platform.DebugRequest
	if r.ContentLength != 0 {
		if err := decode(r, &req); err != nil && !errors.Is(err, io.EOF) {
			fail(w, http.StatusBadRequest, err)
			return
		}
	}
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	v, err := s.provider.CreateDebugContainer(r.Context(), ns, name, req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	detail := "container=" + v.Container + " image=" + v.Image
	if v.Target != "" {
		detail += " target=" + v.Target
	}
	platform.RecordAxiomAudit("pod.debug", ns+"/"+name, detail, s.auditUser(r))
	EmitStateChanged("pod", "debug")
	writeJSON(w, http.StatusCreated, v)
}

// debugCapabilities tells the UI what the debug tooling can do on this server.
func (s *Server) debugCapabilities(w http.ResponseWriter, r *http.Request) {
	c := platform.DebugCapabilities{
		EphemeralContainers: true,
		DebugImage:          platform.DebugImage(),
		NodeShell:           platform.NodeShellEnabled(),
		Interactive:         debugInteractive(),
	}
	if c.NodeShell {
		c.NodeShellNamespace = platform.NodeShellNamespace()
		c.NodeShellTimeoutSec = int(platform.NodeShellTimeout().Seconds())
	}
	writeJSON(w, http.StatusOK, c)
}
