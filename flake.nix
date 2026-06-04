{
  description = "rabbitmq-definitions-loader — generic, config-driven RabbitMQ definitions loader (public init-container image)";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.11";
  };

  outputs = {
    self,
    nixpkgs,
    ...
  }: let
    supportedSystems = ["x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin"];
    forEachSystem = f:
      nixpkgs.lib.genAttrs supportedSystems (system:
        f {
          inherit system;
          pkgs = import nixpkgs {inherit system;};
        });
  in {
    packages = forEachSystem ({pkgs, ...}: rec {
      definitions-loader = pkgs.buildGoModule {
        pname = "definitions-loader";
        version = "0.1.0";
        src = ./.;
        vendorHash = "sha256-ZBMvOkcXA6HgZOwf0lxeqzZn/F8oQtwexPsz43760xY=";
        doCheck = true;
        subPackages = ["cmd/definitions-loader"];
        meta = {
          description = "Generic RabbitMQ definitions loader (typed YAML config; secrets from env)";
          mainProgram = "definitions-loader";
        };
      };

      # PUBLIC linux init-container image — nothing tenant-specific baked in.
      # Push it anywhere public with pleme-io oci-push:
      #   nix run github:pleme-io/substrate#oci-push -- push --tarball ./result \
      #     --registry ghcr.io/pleme-io --image rabbitmq-definitions-loader --tag 0.1.0 ...
      image = pkgs.dockerTools.buildLayeredImage {
        name = "rabbitmq-definitions-loader";
        tag = "0.1.0";
        contents = [definitions-loader];
        config.Entrypoint = ["/bin/definitions-loader"];
      };

      default = definitions-loader;
    });

    checks = forEachSystem ({system, ...}: {
      definitions-loader = self.packages.${system}.definitions-loader;
    });

    devShells = forEachSystem ({pkgs, ...}: {
      default = pkgs.mkShellNoCC {
        packages = with pkgs; [go gopls gotools];
      };
    });
  };
}
