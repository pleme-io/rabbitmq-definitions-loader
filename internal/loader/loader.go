// Package loader loads RabbitMQ definitions into the management API at boot.
//
// It is the typed replacement for the chart's load-definitions.sh: read the
// mounted definitions.json, substitute the read-region federation/shovel upstream
// URI (only when present), wait for the node + management API to be ready, then
// POST the definitions. The substitution uses strings replacement — no sed, so a
// credential-bearing amqps:// URI with any characters is handled safely.
package loader

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Placeholder is the token the read-region-broker module emits into definitions
// in place of the real master upstream URI (which carries credentials).
const Placeholder = "__MASTER_UPSTREAM_URI__"

// Config is the fully-resolved loader configuration (no I/O to construct).
type Config struct {
	DefinitionsFile string        // mounted definitions.json
	BaseURL         string        // http://user:pass@host:port (management API)
	UpstreamURI     string        // MASTER_UPSTREAM_URI; "" => no substitution
	MaxAttempts     int           // total tries before giving up
	RetryInterval   time.Duration // wait between tries
}

// Deps are the injectable side effects — the testability seam. Real
// implementations hit the network/clock; tests supply fakes.
type Deps struct {
	HTTP       *http.Client
	ReadyCheck func(context.Context) error // nil => management-API reachability is the only gate
	Sleep      func(time.Duration)
	Logf       func(string, ...any)
}

// Substitute replaces the upstream-URI placeholder with uri. It is a no-op unless
// both a uri is given AND the placeholder is present, so it is safe to call on
// master definitions (which carry no placeholder).
func Substitute(defs []byte, uri string) []byte {
	if uri == "" || !bytes.Contains(defs, []byte(Placeholder)) {
		return defs
	}
	return bytes.ReplaceAll(defs, []byte(Placeholder), []byte(uri))
}

// Load reads + substitutes the definitions, waits for readiness, and POSTs them.
// Returns nil when the definitions file is absent (nothing to load — matches the
// shell's skip-and-succeed behaviour).
func Load(ctx context.Context, cfg Config, d Deps) error {
	logf := d.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	sleep := d.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}

	raw, err := os.ReadFile(cfg.DefinitionsFile)
	if err != nil {
		if os.IsNotExist(err) {
			logf("definitions file %s not found, skipping", cfg.DefinitionsFile)
			return nil
		}
		return fmt.Errorf("read definitions: %w", err)
	}
	defs := Substitute(raw, cfg.UpstreamURI)
	if len(defs) != len(raw) || !bytes.Equal(defs, raw) {
		logf("rendered master upstream URI into definitions")
	}

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		if d.ReadyCheck != nil {
			if err := d.ReadyCheck(ctx); err != nil {
				logf("attempt %d/%d: cluster not ready: %v", attempt, cfg.MaxAttempts, err)
				sleep(cfg.RetryInterval)
				continue
			}
		}
		if err := getOK(ctx, d.HTTP, cfg.BaseURL+"/api/overview"); err != nil {
			logf("attempt %d/%d: management API not reachable: %v", attempt, cfg.MaxAttempts, err)
			sleep(cfg.RetryInterval)
			continue
		}
		if err := postJSON(ctx, d.HTTP, cfg.BaseURL+"/api/definitions", defs); err != nil {
			logf("attempt %d/%d: posting definitions failed: %v", attempt, cfg.MaxAttempts, err)
			sleep(cfg.RetryInterval)
			continue
		}
		logf("definitions successfully loaded")
		return nil
	}
	return fmt.Errorf("gave up after %d attempts without loading definitions", cfg.MaxAttempts)
}

func getOK(ctx context.Context, c *http.Client, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer drain(resp)
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func postJSON(ctx context.Context, c *http.Client, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer drain(resp)
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}
