package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_DefaultsWhenNoFile(t *testing.T) {
	got, err := Load("", Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Loader.MaxAttempts != 60 || got.Loader.DefinitionsFile != "/etc/definitions/definitions.json" {
		t.Fatalf("prescribed defaults not applied: %+v", got.Loader)
	}
	if !strings.Contains(got.Loader.BaseURL, "guest:guest@127.0.0.1:15672") {
		t.Fatalf("default management endpoint wrong: %s", got.Loader.BaseURL)
	}
}

func TestLoad_YAMLOverlayAndSecrets(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	_ = os.WriteFile(p, []byte("managementPort: 16000\nmaxAttempts: 5\nretrySeconds: 1\nverbose: true\nreadyCommand: /ready.sh\n"), 0o600)

	got, err := Load(p, Secrets{User: "admin", Pass: "s3cr3t", UpstreamURI: "amqps://u:p@master:5671"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Loader.MaxAttempts != 5 || !got.Verbose || got.ReadyCommand != "/ready.sh" {
		t.Fatalf("YAML overlay not applied: %+v", got)
	}
	if !strings.Contains(got.Loader.BaseURL, "admin:s3cr3t@127.0.0.1:16000") {
		t.Fatalf("secrets/port not merged: %s", got.Loader.BaseURL)
	}
	if got.Loader.UpstreamURI != "amqps://u:p@master:5671" {
		t.Fatalf("upstream URI (secret) not carried: %s", got.Loader.UpstreamURI)
	}
}

func TestLoad_RejectsBadPort(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	_ = os.WriteFile(p, []byte("managementPort: 99999\n"), 0o600)
	if _, err := Load(p, Secrets{}); err == nil {
		t.Fatal("expected validation error for out-of-range port")
	}
}
