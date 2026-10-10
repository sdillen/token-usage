{ pkgs, lib, config, inputs, ... }:

{
  # https://devenv.sh/languages/go/
  languages.go = {
    enable = true;
    # Optional eine bestimmte Go-Version festlegen, z. B.:
    # package = pkgs.go_1_26;
  };

  # Zusätzliche Werkzeuge für die Entwicklung
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
