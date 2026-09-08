# Register the CLI on the package set used by the flake outputs.
{self}:
final: _prev: {
  anubis-fetch = import ./anubis-fetch.nix {pkgs = final;};
}
