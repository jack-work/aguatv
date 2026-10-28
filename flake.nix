{
  description = "aguatv: a home screen, a remote and a declarative deploy for Fire TV";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    zanni.url = "github:jack-work/zanni/d7ce694a379729e207d9e0d13e999fcc0a766ec8";
    zanni.inputs.nixpkgs.follows = "nixpkgs";
    dumbtv = {
      url = "github:humblemuzzu/dumbtv";
      flake = false;
    };
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
      zanni,
      dumbtv,
    }:
    {
      nixosModules.hub = import ./nix/hub-module.nix self;

      lib.mkTvs =
        {
          system,
          tvs,
          specialArgs ? { },
        }:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          own = self.packages.${system};
          eval =
            name: module:
            (nixpkgs.lib.evalModules {
              inherit specialArgs;
              modules = [
                ./nix/module.nix
                (import ./nix/dumbtv.nix { inherit dumbtv; })
                { name = nixpkgs.lib.mkDefault name; }
                module
              ];
            }).config;
          configs = builtins.mapAttrs eval tvs;
          planFile = name: pkgs.writeText "aguatv-${name}.json" (builtins.toJSON configs.${name}.plan);
          deployFor =
            name:
            pkgs.writeShellApplication {
              name = "deploy-${name}";
              text = ''
                export AGUATV_CLI=${nixpkgs.lib.getExe own.aguatv}
                export AGUATV_APK=${own.apk}
                export AGUATV_PLAYER_APK=${own.player}/aguatv-player-unsigned.apk
                export AGUATV_REMOTE_PKG=${own.remote}
                exec ${own.cli}/bin/aguatv-deploy "$@" ${planFile name}
              '';
            };
        in
        {
          inherit configs;
          packages = nixpkgs.lib.mapAttrs' (name: _: nixpkgs.lib.nameValuePair "${name}-plan" (planFile name)) tvs;
          apps = builtins.mapAttrs (name: _: {
            type = "app";
            program = nixpkgs.lib.getExe (deployFor name);
          }) tvs;
        };
    }
    // flake-utils.lib.eachSystem [ "x86_64-linux" "aarch64-darwin" "x86_64-darwin" ] (
      system:
      let
        pkgs = import nixpkgs {
          inherit system;
          config = {
            allowUnfree = true;
            android_sdk.accept_license = true;
          };
        };

        version = "0.1.0";
        versionCode = "1";
        minSdk = "28";
        buildTools = "34.0.0";

        sdk =
          (pkgs.androidenv.composeAndroidPackages {
            platformVersions = [ minSdk ];
            buildToolsVersions = [ buildTools ];
            includeEmulator = false;
            includeSystemImages = false;
            includeNDK = false;
          }).androidsdk;
        sdkRoot = "${sdk}/libexec/android-sdk";
        jdk = pkgs.jdk17_headless;

        webPages = zanni.lib.mkBoiledSite {
          inherit pkgs version;
          pname = "aguatv-web";
          src = ./web;
          components = [
            "boil"
            "gesso"
            "phosphor"
            "fontpack"
            "filo"
            "bacchetta"
          ];
        };

        web = pkgs.runCommand "aguatv-web-${version}" { nativeBuildInputs = [ pkgs.python3 ]; } ''
          cp -r ${webPages} $out
          chmod -R u+w $out
          python3 ${./nix/externalize-scripts.py} $out
        '';

        apk = pkgs.stdenv.mkDerivation {
          pname = "aguatv-unsigned";
          inherit version;
          src = ./android;
          nativeBuildInputs = [
            jdk
            pkgs.zip
          ];
          dontConfigure = true;
          buildPhase = ''
            runHook preBuild
            bt=${sdkRoot}/build-tools/${buildTools}
            jar=${sdkRoot}/platforms/android-${minSdk}/android.jar
            mkdir -p work/gen work/classes work/assets
            cp -r ${web} work/assets/web
            chmod -R u+w work/assets
            $bt/aapt2 compile --dir res -o work/res.zip
            $bt/aapt2 link -I $jar --manifest AndroidManifest.xml work/res.zip \
              -A work/assets \
              --min-sdk-version ${minSdk} --target-sdk-version ${minSdk} \
              --version-code ${versionCode} --version-name ${version} \
              --java work/gen -o work/app.apk
            find src work/gen -name '*.java' > work/sources.txt
            javac --release 8 -Xlint:all -Xlint:-options -Werror -encoding UTF-8 \
              -classpath $jar -d work/classes @work/sources.txt
            find work/classes -name '*.class' > work/classes.txt
            $bt/d8 --release --min-api ${minSdk} --lib $jar --output work @work/classes.txt
            (cd work && zip -q -X app.apk classes.dex)
            $bt/zipalign -f -p 4 work/app.apk work/aligned.apk
            runHook postBuild
          '';
          installPhase = ''
            runHook preInstall
            mkdir -p $out
            cp work/aligned.apk $out/aguatv-unsigned.apk
            runHook postInstall
          '';
        };

        cli = pkgs.buildGoModule {
          pname = "aguatv-cli";
          inherit version;
          src = ./cli;
          vendorHash = "sha256-TUbaUoqDZoQTkcOMtoE/FlAiqkWN+x49JeGkDguh2UU=";
          ldflags = [
            "-s"
            "-w"
            "-X main.defaultJar=${pkgs.scrcpy}/share/scrcpy/scrcpy-server"
            "-X main.scrcpyVersion=${pkgs.scrcpy.version}"
            "-X main.defaultAdb=${pkgs.android-tools}/bin/adb"
          ];
          meta.mainProgram = "aguatv-remote";
        };

        hub = pkgs.buildGoModule {
          pname = "aguatv-hub";
          inherit version;
          src = ./cli;
          vendorHash = "sha256-TUbaUoqDZoQTkcOMtoE/FlAiqkWN+x49JeGkDguh2UU=";
          subPackages = [ "cmd/aguatv-hub" ];
          ldflags = [
            "-s"
            "-w"
          ];
          meta.mainProgram = "aguatv-hub";
        };

        remote = pkgs.runCommand "aguatv-remote-${version}" { meta.mainProgram = "aguatv-remote"; } ''
          mkdir -p $out/bin
          ln -s ${cli}/bin/aguatv-remote $out/bin/aguatv-remote
        '';

        player = import ./nix/player.nix { inherit pkgs sdkRoot buildTools minSdk jdk version; };

        aguatv = pkgs.writeShellApplication {
          name = "aguatv";
          runtimeInputs = [
            jdk
            pkgs.android-tools
            pkgs.coreutils
            pkgs.gawk
            pkgs.gnugrep
            pkgs.gnused
          ];
          text = ''
            export AGUATV_APK=${apk}/aguatv-unsigned.apk
            export AGUATV_BUILD_TOOLS=${sdkRoot}/build-tools/${buildTools}
            export AGUATV_REMOTE=${pkgs.lib.getExe remote}
            export AGUATV_PLAYER_APK=${player}/aguatv-player-unsigned.apk
            ${builtins.readFile ./bin/aguatv}
          '';
        };
      in
      {
        packages = {
          inherit
            web
            apk
            cli
            hub
            remote
            player
            aguatv
            ;
          default = pkgs.symlinkJoin {
            name = "aguatv-${version}";
            paths = [
              aguatv
              cli
            ];
          };
        };
        apps = {
          default = flake-utils.lib.mkApp { drv = aguatv; };
          remote = flake-utils.lib.mkApp { drv = remote; };
        };
        checks = nixpkgs.lib.optionalAttrs pkgs.stdenv.isLinux {
          hub-module = import ./nix/hub-test.nix { inherit pkgs self; };
        };
        devShells.default = pkgs.mkShell {
          packages = [
            jdk
            pkgs.android-tools
          ];
          ANDROID_HOME = sdkRoot;
        };
      }
    );
}
