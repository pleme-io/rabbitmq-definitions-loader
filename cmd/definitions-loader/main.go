// Command definitions-loader loads RabbitMQ definitions into a node's management
// API at boot, substituting a federation/shovel upstream URI from a Secret. It is
// GENERIC — nothing about any tenant or environment is baked in; the non-secret
// config is a typed YAML (a ConfigMap), the secrets come from the environment.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/pleme-io/rabbitmq-definitions-loader/internal/config"
	"github.com/pleme-io/rabbitmq-definitions-loader/internal/loader"
)

func main() {
	var configPath, install string
	flag.StringVar(&configPath, "config", envOr("LOADER_CONFIG", "/etc/loader/config.yaml"), "path to the YAML config")
	flag.StringVar(&install, "install", "", "copy this binary to <path> and exit (init-container use)")
	flag.Parse()

	if install != "" {
		if err := installSelf(install); err != nil {
			fmt.Fprintln(os.Stderr, "definitions-loader: install:", err)
			os.Exit(1)
		}
		return
	}

	// secrets come ONLY from the environment (a K8s Secret) — never from YAML.
	resolved, err := config.Load(configPath, config.Secrets{
		User:        os.Getenv("RABBITMQ_USER"),
		Pass:        os.Getenv("RABBITMQ_PASS"),
		UpstreamURI: os.Getenv("MASTER_UPSTREAM_URI"),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "definitions-loader:", err)
		os.Exit(2)
	}

	logf := func(format string, a ...any) { fmt.Fprintf(os.Stderr, "definitions-loader: "+format+"\n", a...) }
	if resolved.Verbose {
		logf("starting, definitions=%s (creds redacted)", resolved.Loader.DefinitionsFile)
	}

	deps := loader.Deps{HTTP: &http.Client{Timeout: 10 * time.Second}, Sleep: time.Sleep, Logf: logf}
	if resolved.ReadyCommand != "" {
		ready := resolved.ReadyCommand
		deps.ReadyCheck = func(ctx context.Context) error {
			return exec.CommandContext(ctx, "/bin/sh", ready).Run()
		}
	}

	if err := loader.Load(context.Background(), resolved.Loader, deps); err != nil {
		logf("%v", err)
		os.Exit(1)
	}
}

func envOr(k, def string) string {
	if v, ok := os.LookupEnv(k); ok {
		return v
	}
	return def
}

func installSelf(dst string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	in, err := os.Open(self)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Chmod(0o755)
}
