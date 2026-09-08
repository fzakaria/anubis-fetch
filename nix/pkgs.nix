# Apply the same overlay for packages, checks, and the development shell.
{
  self,
  system,
}:
import self.inputs.nixpkgs {
  inherit system;
  overlays = [self.overlays.default];
}
