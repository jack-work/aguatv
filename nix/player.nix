{
  pkgs,
  sdkRoot,
  buildTools,
  minSdk,
  jdk,
  version,
  versionCode ? "1",
}:
pkgs.stdenv.mkDerivation {
  pname = "aguatv-player-unsigned";
  inherit version;
  src = ../player;
  nativeBuildInputs = [
    jdk
    pkgs.zip
  ];
  dontConfigure = true;
  buildPhase = ''
    runHook preBuild
    bt=${sdkRoot}/build-tools/${buildTools}
    jar=${sdkRoot}/platforms/android-${minSdk}/android.jar
    mkdir -p work/gen work/classes
    $bt/aapt2 compile --dir res -o work/res.zip
    $bt/aapt2 link -I $jar --manifest AndroidManifest.xml work/res.zip \
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
    cp work/aligned.apk $out/aguatv-player-unsigned.apk
    runHook postInstall
  '';
}
