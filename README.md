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

## Build / image

```bash
nix build .#definitions-loader        # binary (runs go test in-sandbox)
nix build .#packages.x86_64-linux.image   # the public linux OCI image (docker-archive)
```

Push it public with pleme-io's typed [`oci-push`](https://github.com/pleme-io/substrate):

```bash
nix run github:pleme-io/substrate#oci-push -- push \
  --tarball ./result --registry ghcr.io/pleme-io \
  --image rabbitmq-definitions-loader --tag 0.1.0 \
  --dest-user <you> --dest-pass <token>
```

Consumers (e.g. akeyless-environments' rabbitmq-cluster chart) pull the public
image anonymously and supply the YAML config + the K8s Secret.
