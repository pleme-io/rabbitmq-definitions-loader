# rabbitmq-definitions-loader

A **generic, config-driven** RabbitMQ definitions loader, shipped as a **public**
init-container image — nothing about any tenant or environment is baked in.

At boot it reads a mounted `definitions.json`, substitutes a federation/shovel
**upstream URI** (from a Secret) into the `__MASTER_UPSTREAM_URI__` placeholder
(string replacement, not `sed` — credential-safe), waits for the node's management
API to be ready, then POSTs the definitions.

## Config: typed YAML (public) + secrets from env (never baked)

Non-secret config is a typed YAML — a public ConfigMap, validated by
[`config.schema.json`](./config.schema.json):

```yaml
managementHost: 127.0.0.1
managementPort: 15672
definitionsFile: /etc/definitions/definitions.json
maxAttempts: 60
retrySeconds: 3
readyCommand: /etc/rabbitmq/check-cluster-ready.sh   # optional
verbose: false
```

Secrets come **only from the environment** (a K8s Secret) — never from YAML, never
in the image:

| env | meaning |
|---|---|
| `RABBITMQ_USER` / `RABBITMQ_PASS` | management API credentials (default `guest`) |
| `MASTER_UPSTREAM_URI` | the federation/shovel upstream URI substituted into definitions |

`-config <path>` (default `/etc/loader/config.yaml`) selects the YAML.
`-install <path>` copies the binary to a shared volume (init-container pattern).

## Build / publish (substrate Go service-flake)

The flake anchors on substrate's `build/go/service-flake.nix` — standard outputs:

```bash
nix build .#packages.<system>.default                # the CLI binary
nix build '.#packages.<system>."dockerImage:amd64"'  # linux/amd64 OCI image
nix run   .#release                                  # multi-arch ghcr push (forge)
nix flake check                                       # eval + builds
go test ./...                                         # unit tests
```

Publishing is automated (substrate/forge):
- **Image** — every push to `main` → `ghcr.io/pleme-io/rabbitmq-definitions-loader`
  (`<arch>-latest` + `<arch>-<sha>`, content-addressed) via `image-release.yml`
  → substrate `nix-image-auto-release.yml`.
- **Binary** — a `v*` tag → cross-arch binaries on a GitHub Release via
  `binary-release.yml`.

Bump: edit `version` in `flake.nix`, commit, tag `v<new>`, push.

> After the first image publish, set the ghcr package **public** (Package
> settings → visibility) so consumers pull anonymously — no image-pull secret.

Consumers (e.g. akeyless-environments' rabbitmq-cluster chart) pull the public
image and supply the YAML config + the K8s Secret.
