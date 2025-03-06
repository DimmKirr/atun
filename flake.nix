{
  description = "A Nix flake for automationd/atun";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
      in {
        packages.default = pkgs.buildGoModule {
          pname = "atun";
          version = "unstable";
          src = ./.;

          vendorHash = "sha256-T7GCADDGxdffuvIDnpBHYmQTokQALXcoBTLQtBTQOY0=";
          doCheck = false;  # Skip tests
          checkPhase = "";  # Ensure no tests rexun
        };

        devShells.default = pkgs.mkShell {
          buildInputs = [
            pkgs.go
            pkgs.gopls
            pkgs.gotools
            pkgs.nil  # Nix linter
            pkgs.go-task # Task runner
          ];

          shellHook = ''
            export GOROOT=${pkgs.go}/share/go
            export GOBIN=${pkgs.go}/bin
            export PATH=./bin:$PATH
            echo 'Welcome to the atun dev environment'
          '';
        };
      }
    );
}
