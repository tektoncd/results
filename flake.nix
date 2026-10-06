{
  description = "Tekton Results - API for storing Tekton task and pipeline results";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      system = "x86_64-linux";
      pkgs = nixpkgs.legacyPackages.${system};

      mkGoCmd = name: subPath:
        pkgs.buildGoModule {
          pname = name;
          version = "0.0.0-dev";
          src = ./.;
          vendorHash = null;
          subPackages = [ subPath ];
        };
    in
    {
      packages.${system} = {
        api = mkGoCmd "tekton-results-api" "cmd/api";
        watcher = mkGoCmd "tekton-results-watcher" "cmd/watcher";
        retention-policy-agent = mkGoCmd "tekton-results-retention-policy-agent" "cmd/retention-policy-agent";
        tkn-results = mkGoCmd "tkn-results" "cmd/tkn-results";
        default = self.packages.${system}.api;
      };

      checks.${system} = {
        api = self.packages.${system}.api;
        watcher = self.packages.${system}.watcher;
        retention-policy-agent = self.packages.${system}.retention-policy-agent;
        tkn-results = self.packages.${system}.tkn-results;
      };

      devShells.${system} = {
        default = pkgs.mkShell {
          buildInputs = [
            pkgs.go
            pkgs.gopls
            pkgs.protobuf
            pkgs.protoc-gen-go
            pkgs.protoc-gen-go-grpc
          ];
        };
      };
    };
}
