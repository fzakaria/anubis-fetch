{
  description = "anubis-fetch — fetch URLs from behind Anubis / Cloudflare bot-walls";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    treefmt-nix.url = "github:numtide/treefmt-nix";
    treefmt-nix.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs = {
    self,
    nixpkgs,
    flake-utils,
    treefmt-nix,
  }:
    flake-utils.lib.eachDefaultSystem (system: let
      pkgs = nixpkgs.legacyPackages.${system};

      treefmtEval = treefmt-nix.lib.evalModule pkgs ./treefmt.nix;

      # go.mod declares `go 1.26`; pin that toolchain so the sandboxed build
      # never tries to fetch one (GOTOOLCHAIN=local).
      buildGoModule = pkgs.buildGoModule.override {go = pkgs.go_1_26;};

      anubis-fetch = buildGoModule {
        pname = "anubis-fetch";
        version = "0.1.0";
        src = ./.;
        vendorHash = "sha256-+paffObhoUlxvyZ+1yn1PEAbzN9TwohuQ24SsT6otPc=";

        nativeBuildInputs = [pkgs.makeWrapper];

        # The browser fallback drives a headless Chromium; pin the system one so
        # it works out of the box (chromedp otherwise hunts for chrome on PATH).
        postInstall = ''
          wrapProgram $out/bin/anubis-fetch \
            --set CHROMIUM_BIN ${pkgs.chromium}/bin/chromium
        '';

        ldflags = ["-s" "-w"];

        meta = {
          description = "Fetch URLs from behind Anubis' proof-of-work and Cloudflare's fingerprint walls";
          homepage = "https://github.com/fzakaria/anubis-fetch";
          license = pkgs.lib.licenses.mit;
          mainProgram = "anubis-fetch";
        };
      };
    in {
      packages = {
        default = anubis-fetch;
        anubis-fetch = anubis-fetch;
      };

      devShells.default = pkgs.mkShell {
        inputsFrom = [anubis-fetch];
        packages = [
          pkgs.go_1_26
          pkgs.gopls
          pkgs.gotools
          pkgs.chromium
          treefmtEval.config.build.wrapper
        ];
        # So `go run .` / `go test` can exercise the browser fallback in the shell.
        CHROMIUM_BIN = "${pkgs.chromium}/bin/chromium";
      };

      # `nix fmt`
      formatter = treefmtEval.config.build.wrapper;

      # `nix flake check`: builds + runs the Go unit tests (buildGoModule's
      # checkPhase) and verifies formatting.
      checks = {
        package = anubis-fetch;
        formatting = treefmtEval.config.build.check self;
      };
    });
}
