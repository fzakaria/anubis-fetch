# Build the Go runner and pin the upstream executable used by the child process.
{pkgs}:
pkgs.anubis-fetch.overrideAttrs (_: {
  pname = "local-anubis";
  subPackages = ["cmd/local-anubis"];
  postInstall = ''
    wrapProgram $out/bin/local-anubis \
      --set ANUBIS_BIN ${pkgs.anubis}/bin/anubis
  '';
  meta = {
    description = "Run Anubis and a fixed backend on loopback";
    mainProgram = "local-anubis";
  };
})
