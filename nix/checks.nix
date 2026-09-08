{
  self,
  pkgs,
  treefmt,
}: {
  # buildGoModule runs the CLI's Go tests during the package build.
  package = pkgs.anubis-fetch;
  formatting = treefmt.config.build.check self;
}
