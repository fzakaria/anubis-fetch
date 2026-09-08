{pkgs}: let
  inherit (pkgs) lib fetchNpmDeps nodejs npmHooks esbuild brotli zstd binaryen;
  revision = "0c5ef7776309295e8846f10e54b2daff5899e55b";
  version = "1.28.0-pre1-${builtins.substring 0 7 revision}";
  buildGoModule = pkgs.buildGoModule.override {go = pkgs.go_1_26;};
in
  buildGoModule (finalAttrs: {
    pname = "anubis";
    inherit version;
    src = pkgs.fetchFromGitHub {
      owner = "TecharoHQ";
      repo = "anubis";
      rev = revision;
      hash = "sha256-bBvsIGmt/y8xKwUXfWI6Hqx0hLPApULIwPUjAoHRrBI=";
    };
    vendorHash = "sha256-+3u4PphupG5FSKXlCa6S1jBDL+ftneUYXqEjbtOWGIE=";
    subPackages = ["cmd/anubis"];

    npmDeps = fetchNpmDeps {
      name = "anubis-npm-deps-${version}";
      inherit (finalAttrs) src;
      hash = "sha256-wA2z83hIJ0GK5X0oIpzUy5/VUrvv87d0VJBXAIIAyQA=";
    };
    nativeBuildInputs = [nodejs npmHooks.npmConfigHook esbuild brotli zstd binaryen];

    # Go vendoring does not need frontend dependencies or generated assets.
    prePatch = ''
      if [[ $name == *-go-modules ]]; then
        npmConfigHook() { true; }
      fi
    '';
    postPatch = ''
      patchShebangs web/build.sh xess/build.sh lib/challenge/*/build.sh
    '';

    # Build the frontend after npmConfigHook prepares the locked dependencies.
    preBuild = ''
      if [[ $name != *-go-modules ]]; then
        cp -r ${pkgs.anubis-wasm} web/static/wasm
        chmod -R u+w web/static/wasm
        ${pkgs.bashInteractive}/bin/bash wasm/scripts/build_wasm2js.sh
        go generate ./...
        ./web/build.sh
        PATH="$PWD/node_modules/.bin:$PATH" ./xess/build.sh
      fi
    '';
    preCheck = ''
      export DONT_USE_NETWORK=1
    '';
    # Verify the freshly compiled baseline and SIMD modules through wazero.
    postCheck = ''
      go test ./wasm ./lib/challenge/wasm
    '';
    ldflags = ["-s" "-w" "-X=github.com/TecharoHQ/anubis.Version=v${version}"];

    passthru.wasm = pkgs.anubis-wasm;
    meta = {
      description = "Anubis test server with WebAssembly challenges";
      homepage = "https://github.com/TecharoHQ/anubis";
      license = lib.licenses.mit;
      mainProgram = "anubis";
    };
  })
