package platform

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// Durable fleet history. Every persistence interval (and once on shutdown)
// each replica writes a downsampled copy of its in-memory history — 5-minute
// buckets over the last 24 h, at most 288 points per cluster — under its own
// key ("history-<replica id>") of the ConfigMap zyvor-axiom-fleet-history, as
// JSON → gzip → base64. At startup, before the first probe, every replica's
// key is read and merged into the in-memory history. A replica only ever
// writes its own key, so replicas never overwrite each other's data (a
// resourceVersion conflict on the shared object is simply retried); a replica
// started later sees the others' data as of their last save.

const (
	// FleetHistoryConfigMapName is the ConfigMap that holds the snapshots.
	FleetHistoryConfigMapName = "zyvor-axiom-fleet-history"
	// fleetHistoryKeyPrefix prefixes every replica's key.
	fleetHistoryKeyPrefix = "history-"
	// fleetHistoryBucket is the resolution of the persisted history.
	fleetHistoryBucket = 5 * time.Minute
	// fleetHistoryPersistPoints is the most buckets kept per cluster (24 h).
	fleetHistoryPersistPoints = int(fleetHistoryRetention / fleetHistoryBucket)
	// fleetHistoryMaxObjectBytes caps the whole ConfigMap's data (the API
	// server refuses objects over 1 MiB).
	fleetHistoryMaxObjectBytes = 900 * 1024
	// fleetHistorySaveErrLogEvery rate-limits the save-failure log line.
	fleetHistorySaveErrLogEvery = 10 * time.Minute
	// fleetHistoryIOTimeout bounds one load or save.
	fleetHistoryIOTimeout = 10 * time.Second
)

// fleetHistoryLogf is where persistence problems are logged (tests swap it).
var fleetHistoryLogf = log.Printf

// FleetHistoryBucket is one persisted 5-minute bucket: the folded sample plus
// what the checks / failedChecks totals and incident reasons need.
type FleetHistoryBucket struct {
	FleetSample
	Checks int    `json:"n"`
	Failed int    `json:"f,omitempty"`
	Reason string `json:"r,omitempty"`
}

// FleetHistorySnapshot is one replica's persisted history.
type FleetHistorySnapshot struct {
	Version       int                             `json:"v"`
	Replica       string                          `json:"replica"`
	SavedAt       int64                           `json:"savedAt"` // unix seconds
	BucketSeconds int                             `json:"bucketSeconds"`
	Clusters      map[string][]FleetHistoryBucket `json:"clusters"`
}

// newest is the time of the newest bucket (SavedAt when there is none).
func (s FleetHistorySnapshot) newest() int64 {
	var n int64
	for _, bs := range s.Clusters {
		if len(bs) > 0 && bs[len(bs)-1].T > n {
			n = bs[len(bs)-1].T
		}
	}
	if n == 0 {
		n = s.SavedAt
	}
	return n
}

// FleetHistoryStore persists fleet history snapshots.
type FleetHistoryStore interface {
	// Load returns every replica's snapshot.
	Load(ctx context.Context) ([]FleetHistorySnapshot, error)
	// Save writes this replica's snapshot (and drops other replicas' keys
	// whose newest sample is older than 24 h).
	Save(ctx context.Context, snap FleetHistorySnapshot) error
	// Persistent reports whether the data survives a restart.
	Persistent() bool
}

// fleetHistoryBlobs is the raw key/value storage under a store: the
// ConfigMap's data, or a map in memory.
type fleetHistoryBlobs interface {
	read(ctx context.Context) (map[string]string, error)
	// update calls fn with the current data (a private copy fn may change)
	// and writes the result, retrying fn on a write conflict.
	update(ctx context.Context, fn func(data map[string]string) error) error
}

// keyedFleetHistoryStore implements FleetHistoryStore over blobs: one key
// per replica.
type keyedFleetHistoryStore struct {
	blobs      fleetHistoryBlobs
	key        string
	persistent bool
	limit      int
	now        func() time.Time
	trimLog    sync.Once
}

func newKeyedFleetHistoryStore(blobs fleetHistoryBlobs, replica string, persistent bool) *keyedFleetHistoryStore {
	return &keyedFleetHistoryStore{blobs: blobs, key: FleetHistoryReplicaKey(replica), persistent: persistent, limit: fleetHistoryMaxObjectBytes, now: time.Now}
}

func (s *keyedFleetHistoryStore) Persistent() bool { return s.persistent }

func (s *keyedFleetHistoryStore) Load(ctx context.Context) ([]FleetHistorySnapshot, error) {
	data, err := s.blobs.read(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(data))
	for k := range data {
		if strings.HasPrefix(k, fleetHistoryKeyPrefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var out []FleetHistorySnapshot
	var bad []string
	for _, k := range keys {
		snap, err := decodeFleetSnapshot(data[k])
		if err != nil {
			bad = append(bad, k)
			continue
		}
		out = append(out, snap)
	}
	if len(bad) > 0 {
		fleetHistoryLogf("fleet history: ignoring unreadable keys %v", bad)
	}
	return out, nil
}

func (s *keyedFleetHistoryStore) Save(ctx context.Context, snap FleetHistorySnapshot) error {
	staleBefore := s.now().Add(-fleetHistoryRetention).Unix()
	return s.blobs.update(ctx, func(data map[string]string) error {
		others := 0
		for k, v := range data {
			if k == s.key {
				continue
			}
			if strings.HasPrefix(k, fleetHistoryKeyPrefix) {
				// A replica that has not saved for 24 h is gone: its key
				// holds nothing inside the window any more.
				if o, err := decodeFleetSnapshot(v); err == nil && o.newest() < staleBefore {
					delete(data, k)
					continue
				}
			}
			others += len(k) + len(v)
		}
		enc, dropped, err := encodeFleetSnapshot(snap, s.limit-others-len(s.key))
		if err != nil {
			return err
		}
		if dropped > 0 {
			s.trimLog.Do(func() {
				fleetHistoryLogf("fleet history: snapshot over %d KiB, dropped the %d oldest buckets of the largest clusters", s.limit/1024, dropped)
			})
		}
		data[s.key] = enc
		return nil
	})
}

// encodeFleetSnapshot encodes snap as JSON → gzip → base64 in at most budget
// bytes, dropping the oldest buckets of the largest cluster first until it
// fits. It returns the encoding and how many buckets were dropped.
func encodeFleetSnapshot(snap FleetHistorySnapshot, budget int) (string, int, error) {
	if budget <= 0 {
		return "", 0, errors.New("fleet history: no room left in the ConfigMap")
	}
	// Work on a copy: snap's slices are trimmed from the front.
	clusters := make(map[string][]FleetHistoryBucket, len(snap.Clusters))
	total := 0
	for k, v := range snap.Clusters {
		clusters[k] = v
		total += len(v)
	}
	snap.Clusters = clusters
	dropped := 0
	for {
		enc, err := encodeFleetSnapshotRaw(snap)
		if err != nil {
			return "", 0, err
		}
		if len(enc) <= budget {
			return enc, dropped, nil
		}
		if total == 0 {
			return "", 0, errors.New("fleet history: snapshot does not fit even without samples")
		}
		target := int(float64(total) * float64(budget) / float64(len(enc)) * 0.95)
		if target >= total {
			target = total - 1
		}
		for total > target {
			name := largestFleetCluster(clusters)
			clusters[name] = clusters[name][1:]
			total--
			dropped++
		}
	}
}

// largestFleetCluster is the cluster with the most buckets (ties: the first
// name), so trimming is deterministic.
func largestFleetCluster(clusters map[string][]FleetHistoryBucket) string {
	best, n := "", -1
	for k, v := range clusters {
		if len(v) > n || (len(v) == n && k < best) {
			best, n = k, len(v)
		}
	}
	return best
}

func encodeFleetSnapshotRaw(snap FleetHistorySnapshot) (string, error) {
	raw, err := json.Marshal(snap)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if _, err := zw.Write(raw); err != nil {
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func decodeFleetSnapshot(v string) (FleetHistorySnapshot, error) {
	var snap FleetHistorySnapshot
	gz, err := base64.StdEncoding.DecodeString(strings.TrimSpace(v))
	if err != nil {
		return snap, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return snap, err
	}
	// A snapshot is at most ~1 MiB compressed; bound the expansion anyway.
	raw, err := io.ReadAll(io.LimitReader(zr, 64<<20))
	if err != nil {
		return snap, err
	}
	if err := json.Unmarshal(raw, &snap); err != nil {
		return snap, err
	}
	if snap.Version != 1 {
		return snap, fmt.Errorf("unsupported fleet history snapshot version %d", snap.Version)
	}
	return snap, nil
}

// FleetHistoryReplicaKey is the ConfigMap key of a replica: "history-" plus
// the replica id with every character a key may not hold replaced by "-".
func FleetHistoryReplicaKey(replica string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(replica) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	id := b.String()
	if id == "" {
		id = "unknown"
	}
	key := fleetHistoryKeyPrefix + id
	if len(key) > 253 {
		key = key[:253]
	}
	return key
}

// FleetHistoryReplicaID is this process's replica id: POD_NAME, else the
// hostname.
func FleetHistoryReplicaID() string {
	if v := strings.TrimSpace(os.Getenv("POD_NAME")); v != "" {
		return v
	}
	if h, err := os.Hostname(); err == nil && strings.TrimSpace(h) != "" {
		return h
	}
	return "unknown"
}

// ---------------------------------------------------------------------------
// In-memory blobs (demo build and tests)

type memoryFleetHistoryBlobs struct {
	mu   sync.Mutex
	data map[string]string
}

func (m *memoryFleetHistoryBlobs) read(context.Context) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]string, len(m.data))
	for k, v := range m.data {
		out[k] = v
	}
	return out, nil
}

func (m *memoryFleetHistoryBlobs) update(_ context.Context, fn func(map[string]string) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := make(map[string]string, len(m.data)+1)
	for k, v := range m.data {
		next[k] = v
	}
	if err := fn(next); err != nil {
		return err
	}
	m.data = next
	return nil
}

// NewMemoryFleetHistoryStore keeps snapshots in memory: nothing survives the
// process, so it is not Persistent (demo mode and tests).
func NewMemoryFleetHistoryStore(replica string) FleetHistoryStore {
	return newKeyedFleetHistoryStore(&memoryFleetHistoryBlobs{}, replica, false)
}

// ---------------------------------------------------------------------------
// Save / restore on the provider

// fleetPersistWindow is the bucket-aligned window a save covers: 288 buckets
// of 5 minutes ending with the bucket that holds now.
func fleetPersistWindow(now time.Time) (from, to int64) {
	b := int64(fleetHistoryBucket / time.Second)
	end := (now.Unix()/b + 1) * b
	return end - int64(fleetHistoryPersistPoints)*b, end - 1
}

// fleetHistorySnapshot folds the in-memory history of every registered
// cluster into 5-minute buckets over the last 24 h.
func (sp *SwitchableProvider) fleetHistorySnapshot() FleetHistorySnapshot {
	sp.mu.RLock()
	registered := make(map[string]bool, len(sp.order))
	for _, n := range sp.order {
		registered[n] = true
	}
	sp.mu.RUnlock()

	st := sp.fleet()
	st.mu.Lock()
	now := st.now()
	from, to := fleetPersistWindow(now)
	recs := make(map[string][]fleetRecord, len(st.history))
	for name, all := range st.history {
		if !registered[name] {
			continue
		}
		j := sort.Search(len(all), func(k int) bool { return all[k].T >= from })
		if j < len(all) {
			recs[name] = append([]fleetRecord(nil), all[j:]...)
		}
	}
	st.mu.Unlock()

	snap := FleetHistorySnapshot{Version: 1, SavedAt: now.Unix(), BucketSeconds: int(fleetHistoryBucket / time.Second),
		Clusters: make(map[string][]FleetHistoryBucket, len(recs))}
	for name, r := range recs {
		groups := fleetBucketGroups(r, from, to, fleetHistoryPersistPoints)
		buckets := make([]FleetHistoryBucket, 0, len(groups))
		for _, g := range groups {
			b := FleetHistoryBucket{FleetSample: summarizeFleetGroup(g)}
			b.Restored = false
			for _, x := range g {
				b.Checks += x.checks()
				b.Failed += x.failedChecks()
				// The latest reason of the bucket's (worst) state, as
				// incidents show it.
				if x.State == b.State && x.reason != "" && fleetStateRank(x.State) > fleetStateRank(ClusterHealthy) {
					b.Reason = x.reason
				}
			}
			buckets = append(buckets, b)
		}
		snap.Clusters[name] = buckets
	}
	return snap
}

// saveFleetHistory writes the snapshot and records the outcome. A failure is
// logged at most once per 10 minutes and never stops the process.
func (sp *SwitchableProvider) saveFleetHistory(ctx context.Context, store FleetHistoryStore) error {
	snap := sp.fleetHistorySnapshot()
	snap.Replica = FleetHistoryReplicaID()
	ctx, cancel := context.WithTimeout(ctx, fleetHistoryIOTimeout)
	defer cancel()
	err := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic: %v", r)
			}
		}()
		return store.Save(ctx, snap)
	}()
	st := sp.fleet()
	st.mu.Lock()
	now := st.now()
	logIt := false
	if err == nil {
		st.lastSaveOK = now
	} else if st.lastSaveErrLog.IsZero() || now.Sub(st.lastSaveErrLog) >= fleetHistorySaveErrLogEvery {
		st.lastSaveErrLog = now
		logIt = true
	}
	st.mu.Unlock()
	if logIt {
		fleetHistoryLogf("fleet history: save failed (history stays in memory; will retry): %v", err)
	}
	return err
}

// durableLocked: a persistent store is configured and the last save
// succeeded within 3 persistence intervals. st.mu held.
func (st *fleetState) durableLocked(now time.Time) bool {
	if !st.persistent || st.persistInterval <= 0 || st.lastSaveOK.IsZero() {
		return false
	}
	return now.Sub(st.lastSaveOK) <= 3*st.persistInterval
}

// RestoreFleetHistory merges every replica's persisted snapshot into the
// in-memory history: samples are marked restored, clusters no longer
// registered are dropped, identical timestamps are deduplicated (the worse
// state wins) and the 24 h window and per-cluster cap apply. It returns how
// many samples were restored.
func (sp *SwitchableProvider) RestoreFleetHistory(ctx context.Context, store FleetHistoryStore) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, fleetHistoryIOTimeout)
	defer cancel()
	snaps, err := store.Load(ctx)
	if err != nil {
		return 0, err
	}
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	st := sp.fleet()
	st.mu.Lock()
	defer st.mu.Unlock()
	now := st.now()
	cut := now.Add(-fleetHistoryRetention).Unix()
	incoming := map[string][]fleetRecord{}
	var savedAt int64
	for _, snap := range snaps {
		used := false
		for name, buckets := range snap.Clusters {
			if _, ok := sp.entries[name]; !ok {
				continue
			}
			for _, b := range buckets {
				if b.T < cut || b.T > now.Unix() {
					continue
				}
				r := fleetRecord{FleetSample: b.FleetSample, reason: b.Reason, n: max(b.Checks, 1), nFailed: b.Failed}
				r.Restored = true
				r.failed = b.Failed > 0
				incoming[name] = append(incoming[name], r)
				used = true
			}
		}
		if used && snap.SavedAt > savedAt {
			savedAt = snap.SavedAt
		}
	}
	if st.history == nil {
		st.history = map[string][]fleetRecord{}
	}
	restored := 0
	for name, in := range incoming {
		all := append(append([]fleetRecord(nil), st.history[name]...), in...)
		sort.SliceStable(all, func(i, j int) bool { return all[i].T < all[j].T })
		merged := make([]fleetRecord, 0, len(all))
		for _, r := range all {
			if r.T < cut {
				continue
			}
			if n := len(merged); n > 0 && merged[n-1].T == r.T {
				last := &merged[n-1]
				// A live sample beats a restored one; else the worse state.
				if last.Restored && (!r.Restored || fleetStateRank(r.State) > fleetStateRank(last.State)) {
					*last = r
				}
				continue
			}
			merged = append(merged, r)
		}
		if len(merged) > fleetHistoryMaxSamples {
			merged = merged[len(merged)-fleetHistoryMaxSamples:]
		}
		for _, r := range merged {
			if r.Restored {
				restored++
			}
		}
		st.history[name] = merged
	}
	if savedAt > 0 {
		st.restoredFrom = time.Unix(savedAt, 0)
	}
	return restored, nil
}

// StartFleetHistoryPersistence restores the persisted history now (call it
// before StartFleetProber), then saves it every interval and once more when
// ctx is cancelled. The returned channel closes after that final save. A nil
// store or interval <= 0 disables persistence (the channel is closed).
func (sp *SwitchableProvider) StartFleetHistoryPersistence(ctx context.Context, store FleetHistoryStore, interval time.Duration) <-chan struct{} {
	done := make(chan struct{})
	if store == nil || interval <= 0 {
		close(done)
		return done
	}
	st := sp.fleet()
	st.mu.Lock()
	st.persistent = store.Persistent()
	st.persistInterval = interval
	st.mu.Unlock()
	if n, err := sp.RestoreFleetHistory(ctx, store); err != nil {
		fleetHistoryLogf("fleet history: restore failed (starting empty): %v", err)
	} else if n > 0 {
		fleetHistoryLogf("fleet history: restored %d samples", n)
	}
	go func() {
		defer close(done)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				// Flush on shutdown with a fresh context: ctx is already done.
				_ = sp.saveFleetHistory(context.Background(), store)
				return
			case <-t.C:
				_ = sp.saveFleetHistory(ctx, store)
			}
		}
	}()
	return done
}
