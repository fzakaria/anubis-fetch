{
  pkgs,
  src,
  version,
}:
# Build both browser feature levels from the pinned Rust workspace.
pkgs.rustPlatform.buildRustPackage {
  pname = "anubis-wasm";
  inherit version src;
  cargoHash = "sha256-T0lZQyAJLFKrR2kQ2i/WHN15APlEwQ3jBP1w/GUYmuA=";
  nativeBuildInputs = [pkgs.binaryen pkgs.lld];
  buildPhase = ''
    runHook preBuild
    ${pkgs.bashInteractive}/bin/bash wasm/scripts/build_wasm.sh
    runHook postBuild
  '';
  # The WASM modules run inside Anubis, not as native test executables.
  doCheck = false;
  installPhase = ''
    runHook preInstall
    mkdir -p "$out"
    cp -r web/static/wasm/. "$out/"
    runHook postInstall
  '';
}
