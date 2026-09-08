{
  pkgs,
  treefmt,
}: {
  default = pkgs.mkShell {
    inputsFrom = [pkgs.anubis-fetch];
    packages = [
      pkgs.go_1_26
      pkgs.gopls
      pkgs.gotools
      pkgs.chromium
      treefmt.config.build.wrapper
    ];
    # Let go run and go test use the same browser as the packaged CLI.
    CHROMIUM_BIN = "${pkgs.chromium}/bin/chromium";
  };
}
