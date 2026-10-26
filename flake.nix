{
  description = "aguatv: a home screen for Fire TV, drawn by a web page";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    zanni.url = "github:jack-work/zanni";
    zanni.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
      zanni,
    }:
    flake-utils.lib.eachSystem [ "x86_64-linux" "aarch64-darwin" "x86_64-darwin" ] (
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

        web = zanni.lib.mkBoiledSite {
          inherit pkgs version;
          pname = "aguatv-web";
          src = ./web;
          components = [
            "boil"
            "gesso"
            "phosphor"
            "fontpack"
          ];
        };

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

        remote = pkgs.buildGoModule {
          pname = "aguatv-remote";
          inherit version;
          src = ./remote;
          vendorHash = "sha256-TUbaUoqDZoQTkcOMtoE/FlAiqkWN+x49JeGkDguh2UU=";
          ldflags = [
            "-s"
            "-w"
            "-X main.defaultJar=${pkgs.scrcpy}/share/scrcpy/scrcpy-server"
            "-X main.scrcpyVersion=${pkgs.scrcpy.version}"
            "-X main.defaultAdb=${pkgs.android-tools}/bin/adb"
          ];
          postInstall = "mv $out/bin/remote $out/bin/aguatv-remote";
          meta.mainProgram = "aguatv-remote";
        };

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
            ${builtins.readFile ./bin/aguatv}
          '';
        };
      in
      {
        packages = {
          inherit web apk remote aguatv;
          default = aguatv;
        };
        apps.default = flake-utils.lib.mkApp { drv = aguatv; };
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
