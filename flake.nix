{
  description = "rabbitmq-definitions-loader — generic, config-driven RabbitMQ definitions loader (public init-container image + cross-arch CLI)";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.11";
    substrate = {
      url = "github:pleme-io/substrate";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    forge = {
      url = "github:pleme-io/forge";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  # Anchored on substrate's canonical Go service-flake — the standard builder for
  # a public Go CLI tool shipping a cross-arch binary + a linux OCI image. One
  # import produces:
  #   packages.<system>.default                 — the CLI binary
  #   packages.<system>."dockerImage:amd64/arm64"— linux OCI images
  #   apps.<system>.release                      — multi-arch ghcr push (forge)
  #   devShells.<system>.default                 — go/gopls/skopeo/cosign/trivy/syft
  # Bump: edit `version`, `nix flake lock` if deps changed, commit, tag v<new>,
  # push. main push -> image-release.yml; tag push -> binary-release.yml.
  outputs = {
    self,
    nixpkgs,
    substrate,
    forge,
    ...
  }:
    (import "${substrate}/lib/build/go/service-flake.nix" {
      inherit nixpkgs substrate forge;
    }) {
      inherit self;
      serviceName = "rabbitmq-definitions-loader";
      registry = "ghcr.io/pleme-io/rabbitmq-definitions-loader";
      src = self;
      subPackages = ["cmd/rabbitmq-definitions-loader"];
      vendorHash = "sha256-ZBMvOkcXA6HgZOwf0lxeqzZn/F8oQtwexPsz43760xY=";
      version = "0.1.0";
      description = "Generic RabbitMQ definitions loader (typed YAML config; secrets from env)";
      distroless = true;
      # amd64-only for now: the substrate image CI runner (x86_64-linux) can't build
      # the arm64 image config (Required system: aarch64-linux). Consumer nodes (dbk
      # GKE) are amd64. Re-add "arm64" once an arm builder/emulation is in the CI.
      architectures = ["amd64"];
    };
}
