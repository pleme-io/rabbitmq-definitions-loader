package loader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSubstitute(t *testing.T) {
	cases := []struct {
		name, uri, in, want string
	}{
		{"no placeholder is a no-op (master)", "amqps://x", `{"a":1}`, `{"a":1}`},
		{"empty uri is a no-op", "", `{"u":"__MASTER_UPSTREAM_URI__"}`, `{"u":"__MASTER_UPSTREAM_URI__"}`},
		{"substitutes all occurrences", "amqps://u:p@h:5671", `["__MASTER_UPSTREAM_URI__","__MASTER_UPSTREAM_URI__"]`, `["amqps://u:p@h:5671","amqps://u:p@h:5671"]`},
		{"uri with sed-hostile chars (#,/,&) is safe", "amqps://u:p#&/@h", `"__MASTER_UPSTREAM_URI__"`, `"amqps://u:p#&/@h"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := string(Substitute([]byte(c.in), c.uri)); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestLoad_MissingFileSkips(t *testing.T) {
	cfg := Config{DefinitionsFile: filepath.Join(t.TempDir(), "nope.json"), MaxAttempts: 1}
	if err := Load(context.Background(), cfg, Deps{HTTP: http.DefaultClient}); err != nil {
		t.Fatalf("missing file must skip-and-succeed, got %v", err)
	}
}

func TestLoad_SubstitutesAndPostsAfterRetries(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "definitions.json")
	if err := os.WriteFile(f, []byte(`{"parameters":[{"value":{"uri":"__MASTER_UPSTREAM_URI__"}}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var posted string
	overviewHits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/api/overview"):
			overviewHits++
			if overviewHits < 2 { // not reachable on the first attempt
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/api/definitions"):
			b := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(b)
			posted = string(b)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	readyHits := 0
	cfg := Config{DefinitionsFile: f, BaseURL: srv.URL, UpstreamURI: "amqps://u:p@master:5671", MaxAttempts: 5, RetryInterval: time.Millisecond}
	deps := Deps{
		HTTP:  srv.Client(),
		Sleep: func(time.Duration) {},
		ReadyCheck: func(context.Context) error {
			readyHits++
			if readyHits < 2 { // cluster not ready on the first attempt
				return errContext
			}
			return nil
		},
	}
	if err := Load(context.Background(), cfg, deps); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !strings.Contains(posted, "amqps://u:p@master:5671") || strings.Contains(posted, Placeholder) {
		t.Fatalf("posted definitions not substituted: %s", posted)
	}
	if readyHits < 2 || overviewHits < 2 {
		t.Fatalf("expected retries: ready=%d overview=%d", readyHits, overviewHits)
	}
}

func TestLoad_GivesUpAfterMaxAttempts(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "definitions.json")
	_ = os.WriteFile(f, []byte(`{}`), 0o600)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable) // never reachable
	}))
	defer srv.Close()
	cfg := Config{DefinitionsFile: f, BaseURL: srv.URL, MaxAttempts: 3, RetryInterval: time.Millisecond}
	err := Load(context.Background(), cfg, Deps{HTTP: srv.Client(), Sleep: func(time.Duration) {}})
	if err == nil {
		t.Fatal("expected failure after max attempts")
	}
}

var errContext = &staticErr{"not ready"}

type staticErr struct{ s string }

func (e *staticErr) Error() string { return e.s }
