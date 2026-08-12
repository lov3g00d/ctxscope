{
  description = "ctxscope";
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    nixpkgs-darwin-x86.url = "github:NixOS/nixpkgs/nixpkgs-26.05-darwin";
  };

  outputs =
    {
      nixpkgs,
      nixpkgs-darwin-x86,
      ...
    }:
    let
      supportedSystems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forEachSystem = nixpkgs.lib.genAttrs supportedSystems;
      nixpkgsFor = system: if system == "x86_64-darwin" then nixpkgs-darwin-x86 else nixpkgs;
    in
    {
      devShells = forEachSystem (
        system:
        let
          pkgs = import (nixpkgsFor system) { inherit system; };
        in
        {
          default = pkgs.mkShell {
            packages = [
              pkgs.go
            ];
          };
        }
      );
    };
}
