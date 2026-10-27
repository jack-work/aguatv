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
    pkgs.zbar
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
    mkdir -p work/test
    javac --release 8 -Xlint:all -Xlint:-options -Werror -encoding UTF-8 -d work/test \
      src/tv/aguatv/player/Fence.java test/tv/aguatv/player/FenceTest.java
    java -cp work/test tv.aguatv.player.FenceTest
    javac --release 8 -Xlint:all -Xlint:-options -Werror -encoding UTF-8 -d work/test \
      src/tv/aguatv/player/QrCode.java test/tv/aguatv/player/QrCodeTest.java
    mkdir -p work/qr
    java -cp work/test tv.aguatv.player.QrCodeTest work/qr
    for t in work/qr/*.txt; do
      got=$(zbarimg -q --raw "''${t%.txt}.pbm" || true)
      if [ "$got" = "$(cat "$t")" ]; then echo "ok   zbar reads code $(basename "$t" .txt)"
      else echo "FAIL code $(basename "$t" .txt) decodes as: $got"; exit 1; fi
    done
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
