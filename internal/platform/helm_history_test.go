package platform

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func encodeRel(t *testing.T, rel map[string]any, b64 bool) []byte {
	t.Helper()
	j, _ := json.Marshal(rel)
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write(j)
	_ = zw.Close()
	if b64 {
		return []byte(base64.StdEncoding.EncodeToString(buf.Bytes()))
	}
	return buf.Bytes()
}

func relFixture(version int, status, manifest string, cfg map[string]any, hooks int) map[string]any {
	h := []any{}
	for i := 0; i < hooks; i++ {
		h = append(h, map[string]any{"name": "h"})
	}
	return map[string]any{
		"name": "web", "namespace": "apps", "version": version, "manifest": manifest, "config": cfg, "hooks": h,
		"info":  map[string]any{"status": status, "description": "Upgrade complete", "last_deployed": "2026-01-02T03:04:05Z"},
		"chart": map[string]any{"metadata": map[string]any{"name": "web", "version": "1.2.3", "appVersion": "9.9"}},
	}
}

func TestDecodeHelmRevisionAndRedaction(t *testing.T) {
	man := "kind: Secret\ndata:\n  foo: YmFy\nstringData:\n  x: y\n---\nkind: Deployment\nenv:\n  - DB_PASSWORD: hunter2\n  name: ok\n"
	cfg := map[string]any{"replicas": 2, "db": map[string]any{"password": "p", "host": "h"}, "apiToken": "t"}
	for _, b64 := range []bool{true, false} {
		d, err := decodeHelmRevision(encodeRel(t, relFixture(3, "deployed", man, cfg, 1), b64))
		if err != nil {
			t.Fatal(err)
		}
		if d.Revision != 3 || d.ChartVersion != "1.2.3" || d.AppVersion != "9.9" || d.Description == "" || d.Hooks != 1 {
			t.Fatalf("bad decode: %+v", d.HelmRevision)
		}
		for _, leak := range []string{"hunter2", "YmFy", "\"p\"", "\"t\""} {
			if strings.Contains(d.Values+d.Manifest, leak) {
				t.Fatalf("leaked %s:\n%s\n%s", leak, d.Values, d.Manifest)
			}
		}
		if !strings.Contains(d.Values, "db.host: \"h\"") || !strings.Contains(d.Manifest, "name: ok") {
			t.Fatalf("over-redacted: %s | %s", d.Values, d.Manifest)
		}
	}
	if _, err := decodeHelmRevision([]byte("garbage")); err == nil {
		t.Fatal("expected error")
	}
}

func TestDiffAndPreview(t *testing.T) {
	a := HelmRevisionData{HelmRevision: HelmRevision{Revision: 4, Status: "deployed"}, Values: "a: 1", Manifest: "kind: Deployment\nimage: v2"}
	b := HelmRevisionData{HelmRevision: HelmRevision{Revision: 2, Status: "superseded"}, Values: "a: 1", Manifest: "kind: Deployment\nimage: v1\n---\nkind: PersistentVolumeClaim", Hooks: 1}
	d := buildHelmDiff("ns", "web", a, b)
	if !d.Changed || helmDiffChanged(d.Values) || !helmDiffChanged(d.Manifest) {
		t.Fatalf("diff: %+v", d)
	}
	p := buildHelmRollbackPreview("ns", "web", a, b)
	if p.Target != 2 || p.Current != 4 || len(p.Warnings) != 2 || p.ConfirmName != "web" {
		t.Fatalf("preview: %+v", p)
	}
	if checkHelmRollbackConfirm(p, "") == nil || checkHelmRollbackConfirm(p, "WEB") == nil || checkHelmRollbackConfirm(p, "web") != nil {
		t.Fatal("confirm")
	}
	if buildHelmDiff("n", "w", a, a).Changed {
		t.Fatal("identical must not change")
	}
}

func TestRevisionPicking(t *testing.T) {
	revs := []HelmRevision{{Revision: 1, Status: "superseded"}, {Revision: 2, Status: "superseded"}, {Revision: 3, Status: "deployed"}, {Revision: 4, Status: "failed"}}
	if liveHelmRevision(revs) != 3 || previousHelmRevision(revs, 3) != 2 || previousHelmRevision(revs, 1) != 0 {
		t.Fatal("picking")
	}
}

// A demo rollback keeps the later revisions in history (as real Helm does), so the release can be
// rolled forward again; the live revision is always the only "deployed" one.
func TestDemoHelmRollbackKeepsLaterRevisions(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	const ns, name = "ingress-nginx", "ingress-nginx"
	before, err := d.ListHelmRevisions(ctx, ns, name)
	if err != nil || len(before) < 3 {
		t.Fatalf("history before: %v %v", before, err)
	}
	live := len(before)
	t.Cleanup(func() { _, _ = d.HelmRollback(ctx, ns, name, live) })

	if _, err := d.HelmRollback(ctx, ns, name, 2); err != nil {
		t.Fatal(err)
	}
	after, _ := d.ListHelmRevisions(ctx, ns, name)
	if len(after) < live {
		t.Fatalf("history shrank after rollback: %d -> %d", live, len(after))
	}
	deployed := 0
	for _, r := range after {
		if r.Status == "deployed" {
			deployed++
			if r.Revision != 2 {
				t.Fatalf("deployed revision = %d, want 2", r.Revision)
			}
		}
	}
	if deployed != 1 {
		t.Fatalf("deployed revisions = %d, want 1", deployed)
	}
	if _, err := d.PreviewHelmRollback(ctx, ns, name, live); err != nil {
		t.Fatalf("cannot roll forward again: %v", err)
	}
}
