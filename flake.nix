{
  description = "anubis-fetch — fetch URLs from behind Anubis / Cloudflare bot-walls";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    treefmt-nix.url = "github:numtide/treefmt-nix";
    treefmt-nix.inputs.nixpkgs.follows = "nixpkgs";
  };

  # Package definitions and development tools live under nix/.
  outputs = {
    self,
    nixpkgs,
    flake-utils,
    ...
  }:
    {
      overlays.default = import ./nix/overlay.nix;
    }
    // flake-utils.lib.eachDefaultSystem (system: let
      pkgs = import ./nix/pkgs.nix {inherit self system;};
      treefmt = import ./nix/formatter.nix {inherit self pkgs;};
    in {
      packages = {
        inherit (pkgs) anubis-fetch anubis local-anubis;
        default = pkgs.anubis-fetch;
      };
      devShells = import ./nix/dev-shells.nix {inherit pkgs treefmt;};
      checks = import ./nix/checks.nix {inherit self pkgs treefmt;};
      formatter = treefmt.config.build.wrapper;
    });
}
