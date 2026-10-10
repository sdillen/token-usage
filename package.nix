# The token-usage package: built with buildGoModule so `go install` and the
# Nix build share one source of truth. The version is embedded into the
# binary via -ldflags; `token-usage --version` prints it.
#
# Note: the parameter is called `gitCommit`, NOT `commit` — `pkgs.commit`
# exists in nixpkgs, so callPackage would auto-inject that package into a
# `commit` parameter and the store path would end up in the ldflags.
{
  pkgs,
  lib,
  version ? "0.1.0",
  gitCommit ? "unknown",
}:

pkgs.buildGoModule {
  pname = "token-usage";
  inherit version;

  src = lib.cleanSourceWith {
    src = ./.;
    filter = name: type:
      let
        base = baseNameOf (toString name);
      in
      !(builtins.elem base [
        ".git"
        ".devenv"
        ".direnv"
        "bin"
        "dist"
        "result"
      ]);
  };

  # The module sources are fetched and hashed at build time; update with
  # `nix build` after changing go.mod/go.sum.
  vendorHash = "sha256-nC2IuGRWJOYn1ZLKcTM2eskC52Badcgl2kud7oQ4wbM=";

  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
  ];

  subPackages = [ "cmd/token-usage" ];

  # No cgo, no runtime git binary: pure-Go build.
  env.CGO_ENABLED = "0";

  meta = {
    description = "Terminal UI for Claude Code and pi token usage and spend";
    homepage = "https://github.com/sdillen/token-usage";
    mainProgram = "token-usage";
  };
}
