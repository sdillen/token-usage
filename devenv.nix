{ pkgs, lib, config, inputs, ... }:

{
  # https://devenv.sh/languages/go/
  languages.go = {
    enable = true;
    # Optionally pin a specific Go version, e.g.:
    # package = pkgs.go_1_26;
  };

  # Additional development tools
  packages = [
    pkgs.golangci-lint
  ];

  enterShell = ''
    go version
  '';

  enterTest = ''
    go version | grep --color=auto "go1"
    golangci-lint version
  '';
}
