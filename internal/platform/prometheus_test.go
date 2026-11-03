package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPromQLBuildersEscape(t *testing.T) {
	evil := `a"} or vector(1) #`
	for _, q := range []string{promCPUP95Query(evil, "7d"), promMemP99Query(evil, "7d")} {
		if !strings.Contains(q, `namespace="a\"} or vector(1) #"`) {
			t.Fatalf("namespace not escaped: %s", q)
		}
		// The only unescaped quotes are the label literals' own delimiters.
		if strings.Contains(q, `"a"}`) {
			t.Fatalf("literal closed early: %s", q)
		}
	}
	if got := promQuote("back\\slash\nnew"); got != `"back\\slash\nnew"` {
		t.Fatalf("promQuote: %s", got)
	}
	all := promCPUP95Query("", "30d")
	if !strings.Contains(all, `namespace!=""`) || !strings.Contains(all, `[30d:5m]`) || !strings.Contains(all, `quantile_over_time(0.95`) ||
		!strings.Contains(all, `container!="POD"`) || !strings.Contains(all, `max by (namespace, pod, container)`) {
		t.Fatalf("cpu query: %s", all)
	}
	mem := promMemP99Query("team-a", "1d")
	if mem != `max by (namespace, pod, container) (quantile_over_time(0.99, container_memory_working_set_bytes{container!="",container!="POD",namespace="team-a"}[1d]))` {
		t.Fatalf("mem query: %s", mem)
	}
}

func TestNewPrometheusClientValidation(t *testing.T) {
	if c, err := NewPrometheusClient("  ", "t"); c != nil || err != nil {
		t.Fatal("empty URL means not configured")
	}
	for _, u := range []string{"ftp://prom", "file:///etc/passwd", "http://", "https://user:pw@prom.example.com", "::bad"} {
		if c, err := NewPrometheusClient(u, "secret-token"); err == nil || c != nil {
			t.Errorf("%q accepted", u)
		} else if strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), "pw@") {
			t.Errorf("error leaks: %v", err)
		}
	}
	c, err := NewPrometheusClient("http://prometheus.monitoring.svc:9090/prom/?x=1#f", "")
	if err != nil || c.base.String() != "http://prometheus.monitoring.svc:9090/prom" {
		t.Fatalf("in-cluster URL: %v %v", c, err)
	}
	if tr, ok := c.http.Transport.(*http.Transport); !ok || tr.Proxy != nil {
		t.Fatal("proxy environment must be ignored")
	}
}

const promTestToken = "tok-ABC-123-very-secret"

func promTestClient(t *testing.T, h http.HandlerFunc) (*PrometheusClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := NewPrometheusClient(srv.URL+"/base", promTestToken)
	if err != nil {
		t.Fatal(err)
	}
	return c, srv
}

func assertSafeErr(t *testing.T, err error, srv *httptest.Server, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want error containing %q", want)
	}
	msg := err.Error()
	if !strings.Contains(msg, want) {
		t.Fatalf("error %q, want it to contain %q", msg, want)
	}
	if strings.Contains(msg, promTestToken) || strings.Contains(msg, srv.URL) || strings.Contains(msg, "127.0.0.1") {
		t.Fatalf("error leaks URL or token: %q", msg)
	}
}

func TestPrometheusClientSuccessAndToken(t *testing.T) {
	var gotAuth, gotPath, gotQuery string
	c, _ := promTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath, gotQuery = r.Header.Get("Authorization"), r.URL.Path, r.URL.Query().Get("query")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[
			{"metric":{"namespace":"a","pod":"p","container":"c"},"value":[1700000000.1,"0.5"]},
			{"metric":{"namespace":"a","pod":"q","container":"c"},"value":[1700000000.1,"NaN"]}]}}`))
	})
	out, err := c.Query(context.Background(), `up{job="x"}`)
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer "+promTestToken || gotPath != "/base/api/v1/query" || gotQuery != `up{job="x"}` {
		t.Fatalf("request: %q %q %q", gotAuth, gotPath, gotQuery)
	}
	if len(out) != 1 || out[0].Value != 0.5 || out[0].Labels["pod"] != "p" {
		t.Fatalf("samples %+v", out)
	}
}

func TestPrometheusClientEmptyResult(t *testing.T) {
	c, _ := promTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	})
	out, err := c.Query(context.Background(), "up")
	if err != nil || len(out) != 0 {
		t.Fatalf("%v %v", out, err)
	}
}

func TestPrometheusClientFailures(t *testing.T) {
	cases := []struct {
		name string
		h    http.HandlerFunc
		want string
	}{
		{"non-200", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "internal "+r.Header.Get("Authorization"), 500)
		}, "HTTP 500"},
		{"non-200 with errorType", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"status":"error","errorType":"bad_data","error":"parse error near ` + promTestToken + `"}`))
		}, "HTTP 400 (bad_data)"},
		{"status error", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"status":"error","errorType":"timeout","error":"x"}`))
		}, "query failed (timeout)"},
		{"malformed", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"status":`)) }, "malformed"},
		{"bad value", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1,2]}]}}`))
		}, "malformed"},
		{"matrix", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
		}, "expected a vector"},
		{"huge body", func(w http.ResponseWriter, r *http.Request) {
			chunk := strings.Repeat("x", 1<<20)
			for i := 0; i < 9; i++ {
				_, _ = w.Write([]byte(chunk))
			}
		}, "exceeds 8 MiB"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, srv := promTestClient(t, tc.h)
			_, err := c.Query(context.Background(), "up")
			assertSafeErr(t, err, srv, tc.want)
		})
	}
}

func TestPrometheusClientRedirects(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("redirect to another host was followed (auth %q)", r.Header.Get("Authorization"))
	}))
	defer other.Close()
	c, srv := promTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/api/v1/query", http.StatusFound)
	})
	_, err := c.Query(context.Background(), "up")
	assertSafeErr(t, err, srv, "another host")

	// A same-host redirect is followed.
	c2, _ := promTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/base/api/v1/query" {
			http.Redirect(w, r, "/moved/api/v1/query?"+r.URL.RawQuery, http.StatusFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+promTestToken {
			t.Errorf("token not kept on same-host redirect")
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	})
	if _, err := c2.Query(context.Background(), "up"); err != nil {
		t.Fatalf("same-host redirect: %v", err)
	}
}

func TestPrometheusClientTimeout(t *testing.T) {
	release := make(chan struct{})
	c, srv := promTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	defer close(release)
	c.timeout = 100 * time.Millisecond
	start := time.Now()
	_, err := c.Query(context.Background(), "up")
	assertSafeErr(t, err, srv, "timed out")
	if time.Since(start) > 5*time.Second {
		t.Fatal("timeout not enforced")
	}
}

func TestPrometheusClientUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	c, err := NewPrometheusClient(srv.URL, promTestToken)
	if err != nil {
		t.Fatal(err)
	}
	srv.Close()
	_, err = c.Query(context.Background(), "up")
	assertSafeErr(t, err, srv, "unreachable")
	var nilClient *PrometheusClient
	if _, err := nilClient.Query(context.Background(), "up"); err == nil {
		t.Fatal("nil client must fail")
	}
}
