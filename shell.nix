{ pkgs ? import <nixpkgs> {} }:

pkgs.mkShell {
  buildInputs = with pkgs; [
    go
    go-tools # staticcheck

    postgresql
    imagemagick
    exiv2

    # keep this line if you use bash
    bashInteractive
  ];
}
