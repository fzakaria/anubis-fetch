{
  self,
  pkgs,
  treefmt,
}: {
  # buildGoModule runs the CLI's Go tests during the package build.
  package = pkgs.anubis-fetch;
  anubis = pkgs.anubis;

  # Exercise each challenge against Chromium and inspect the served WASM modules.
  local-anubis = pkgs.anubis-fetch.overrideAttrs (_: {
    pname = "anubis-fetch-integration";
    ANUBIS_TEST_SERVER = "${pkgs.local-anubis}/bin/local-anubis";
    ANUBIS_FETCH = "${pkgs.anubis-fetch}/bin/anubis-fetch";
  });
  formatting = treefmt.config.build.check self;
}
