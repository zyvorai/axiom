package platform

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
)

// Multi-pod workload log streaming (Workstream B3). The merge logic is
// build-tag free so the Demo and Kubernetes providers share it and it can be
// unit-tested without a cluster.

const (
	// MaxWorkloadLogStreams bounds how many pod/container log streams one
	// request fans out to; a 500-replica Deployment must not open 500 watches.
	MaxWorkloadLogStreams = 40
	// defaultWorkloadLogTail is the per-container tail when none is requested.
	defaultWorkloadLogTail = 100
	// maxMergedLogLines caps a non-follow response (newest lines are kept).
	maxMergedLogLines = 20000
	maxLogLineBytes   = 256 * 1024
)

// WorkloadLogOptions selects what a workload log request returns.
type WorkloadLogOptions struct {
	Container string // restrict to this container name ("" = every app container)
	Follow    bool
	Tail      int64 // lines per container; <=0 means the default
	Previous  bool  // logs of the previous (terminated) container instance
}

func (o WorkloadLogOptions) tail() int64 {
	if o.Tail <= 0 {
		return defaultWorkloadLogTail
	}
	return o.Tail
}

type labeledLogStream struct {
	label string // "pod/container"
	rc    io.ReadCloser
}

type mergedLogs struct {
	io.Reader
	once    sync.Once
	closers []io.Closer
}

func (m *mergedLogs) Close() error {
	m.once.Do(func() {
		for _, c := range m.closers {
			_ = c.Close()
		}
	})
	return nil
}

// timestampOf returns the RFC3339 timestamp prefix of a Kubernetes log line
// ("2026-01-02T03:04:05.123456789Z message") or "" when absent. RFC3339Nano in
// UTC sorts lexically, which is all the non-follow merge needs.
func timestampOf(line string) string {
	i := strings.IndexByte(line, ' ')
	if i < 20 || !strings.Contains(line[:i], "T") {
		return ""
	}
	return line[:i]
}

// mergeLogStreams interleaves several labelled log streams into one stream
// whose lines are prefixed "[pod/container] ".
//
//   - follow=false: every stream is read to EOF and the lines are sorted by
//     their timestamp, so the result is a true chronological merge.
//   - follow=true: lines are forwarded as they arrive (each stream's own
//     backlog first), so ordering across pods is arrival order. The stream
//     ends when every source ends or the caller closes it.
//
// Closing the returned reader closes every source stream.
func mergeLogStreams(streams []labeledLogStream, follow bool) io.ReadCloser {
	pr, pw := io.Pipe()
	closers := []io.Closer{pr}
	for _, s := range streams {
		closers = append(closers, s.rc)
	}
	out := &mergedLogs{Reader: pr, closers: closers}
	if follow {
		go func() {
			var wg sync.WaitGroup
			var mu sync.Mutex
			for _, s := range streams {
				wg.Add(1)
				go func(s labeledLogStream) {
					defer wg.Done()
					sc := newLogScanner(s.rc)
					for sc.Scan() {
						mu.Lock()
						_, err := fmt.Fprintf(pw, "[%s] %s\n", s.label, sc.Text())
						mu.Unlock()
						if err != nil {
							return
						}
					}
				}(s)
			}
			wg.Wait()
			_ = pw.Close()
		}()
		return out
	}
	go func() {
		type entry struct{ ts, text string }
		locals := make([][]entry, len(streams))
		var wg sync.WaitGroup
		for i, s := range streams {
			wg.Add(1)
			go func(i int, s labeledLogStream) {
				defer wg.Done()
				sc := newLogScanner(s.rc)
				for sc.Scan() {
					line := sc.Text()
					locals[i] = append(locals[i], entry{ts: timestampOf(line), text: "[" + s.label + "] " + line})
				}
			}(i, s)
		}
		wg.Wait()
		// Flatten in stream order so equal timestamps sort deterministically.
		var entries []entry
		for _, l := range locals {
			entries = append(entries, l...)
		}
		sort.SliceStable(entries, func(i, j int) bool { return entries[i].ts < entries[j].ts })
		if len(entries) > maxMergedLogLines {
			entries = entries[len(entries)-maxMergedLogLines:]
		}
		w := bufio.NewWriter(pw)
		for _, e := range entries {
			_, _ = w.WriteString(e.text)
			_ = w.WriteByte('\n')
		}
		_ = w.Flush()
		_ = pw.Close()
	}()
	return out
}

func newLogScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), maxLogLineBytes)
	return sc
}

// SupportedLogWorkloadKind reports whether WorkloadLogs can aggregate logs for
// the workload kind.
func SupportedLogWorkloadKind(kind string) bool {
	switch strings.ToLower(kind) {
	case "deployment", "statefulset", "daemonset", "replicaset", "job":
		return true
	}
	return false
}
