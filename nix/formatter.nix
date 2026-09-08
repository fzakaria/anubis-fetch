{
  self,
  pkgs,
}:
self.inputs.treefmt-nix.lib.evalModule pkgs {
  projectRootFile = "flake.nix";
  programs = {
    gofmt.enable = true;
    alejandra.enable = true;
  };
}
