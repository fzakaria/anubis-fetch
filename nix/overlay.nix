# Register the CLI and the pinned upstream test server on one package set.
final: _prev: {
  anubis-fetch = import ./anubis-fetch.nix {pkgs = final;};
  anubis-wasm = import ./anubis-wasm.nix {
    pkgs = final;
    inherit (final.anubis) src version;
  };
  anubis = import ./anubis.nix {pkgs = final;};
}
