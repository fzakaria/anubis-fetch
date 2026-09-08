# Build the CLI and pin Chromium for the browser fallback.
{pkgs}: let
  # Keep the Go toolchain aligned with go.mod.
  buildGoModule = pkgs.buildGoModule.override {go = pkgs.go_1_26;};
in
  buildGoModule {
    pname = "anubis-fetch";
    version = "0.1.0";
    # Keep test-server and packaging changes out of the CLI source closure.
    src = pkgs.lib.fileset.toSource {
      root = ../.;
      fileset =
        pkgs.lib.fileset.fileFilter
        (file: file.hasExt "go" || builtins.elem file.name ["go.mod" "go.sum"])
        ../.;
    };
    vendorHash = "sha256-+paffObhoUlxvyZ+1yn1PEAbzN9TwohuQ24SsT6otPc=";

    subPackages = ["."];

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
  }
