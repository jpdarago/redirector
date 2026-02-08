{ pkgs, ... }:

{
  languages.go.enable = true;

  processes.server.exec = "go run .";

  enterShell = ''
    echo "redirector dev environment"
    go version
  '';
}
