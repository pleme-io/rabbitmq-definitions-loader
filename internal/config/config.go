// Package config is the typed, schema-validated configuration for the
// definitions-loader. The NON-SECRET fields load from a YAML file (a public
// ConfigMap); SECRETS (rabbitmq password, master upstream URI) load from the
// environment (a K8s Secret) and never appear in the YAML or the image. The
// shape mirrors the shikumi tier model: a prescribed default floor, overlaid by
// the authored YAML, validated, then merged with the runtime secrets.
package config

import (
	"fmt"
	"net/url"
	"os"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/pleme-io/rabbitmq-definitions-loader/internal/loader"
)

// File is the non-secret config, authored as one YAML document (ConfigMap key).
type File struct {
	ManagementHost  string `json:"managementHost,omitempty"`
	ManagementPort  int    `json:"managementPort,omitempty"`
	DefinitionsFile string `json:"definitionsFile,omitempty"`
	MaxAttempts     int    `json:"maxAttempts,omitempty"`
	RetrySeconds    int    `json:"retrySeconds,omitempty"`
	ReadyCommand    string `json:"readyCommand,omitempty"`
	Verbose         bool   `json:"verbose,omitempty"`
}

// Secrets come from the environment (a K8s Secret) — never from YAML, never baked.
type Secrets struct {
	User        string // RABBITMQ_USER (default: guest)
	Pass        string // RABBITMQ_PASS (default: guest)
	UpstreamURI string // MASTER_UPSTREAM_URI (empty => no substitution)
}

// Resolved is the fully-merged runtime config the loader consumes.
type Resolved struct {
	Loader       loader.Config
	ReadyCommand string
	Verbose      bool
}

// prescribed is the Tier-2 default floor: a usable config with no YAML at all.
func prescribed() File {
	return File{
		ManagementHost:  "127.0.0.1",
		ManagementPort:  15672,
		DefinitionsFile: "/etc/definitions/definitions.json",
		MaxAttempts:     60,
		RetrySeconds:    3,
	}
}

// Load reads the YAML config at path (empty/absent => prescribed defaults),
// overlays it on the defaults, validates, then folds in the env secrets.
func Load(path string, sec Secrets) (Resolved, error) {
	f := prescribed()
	if path != "" {
		b, err := os.ReadFile(path)
		switch {
		case err == nil:
			if err := yaml.Unmarshal(b, &f); err != nil {
				return Resolved{}, fmt.Errorf("parse config %s: %w", path, err)
			}
		case os.IsNotExist(err):
			// no file => prescribed defaults (a bare deployment is valid)
		default:
			return Resolved{}, fmt.Errorf("read config %s: %w", path, err)
		}
	}
	if err := f.validate(); err != nil {
		return Resolved{}, err
	}

	user := orDefault(sec.User, "guest")
	pass := orDefault(sec.Pass, "guest")
	base := url.URL{
		Scheme: "http",
		User:   url.UserPassword(user, pass),
		Host:   fmt.Sprintf("%s:%d", f.ManagementHost, f.ManagementPort),
	}
	return Resolved{
		Loader: loader.Config{
			DefinitionsFile: f.DefinitionsFile,
			BaseURL:         base.String(),
			UpstreamURI:     sec.UpstreamURI,
			MaxAttempts:     f.MaxAttempts,
			RetryInterval:   time.Duration(f.RetrySeconds) * time.Second,
		},
		ReadyCommand: f.ReadyCommand,
		Verbose:      f.Verbose,
	}, nil
}

func (f File) validate() error {
	if f.ManagementPort < 1 || f.ManagementPort > 65535 {
		return fmt.Errorf("config: managementPort must be 1..65535 (got %d)", f.ManagementPort)
	}
	if f.MaxAttempts < 1 {
		return fmt.Errorf("config: maxAttempts must be >= 1 (got %d)", f.MaxAttempts)
	}
	if f.RetrySeconds < 0 {
		return fmt.Errorf("config: retrySeconds must be >= 0 (got %d)", f.RetrySeconds)
	}
	if f.DefinitionsFile == "" {
		return fmt.Errorf("config: definitionsFile must be non-empty")
	}
	return nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
