# treefmt-nix configuration: one formatter entrypoint for the whole tree.
# `nix fmt` formats; `nix flake check` verifies formatting.
{
  projectRootFile = "flake.nix";
  programs = {
    gofmt.enable = true;
    alejandra.enable = true;
  };
}
