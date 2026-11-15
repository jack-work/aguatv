{
  pkgs,
  sdkRoot,
  buildTools,
  jdk,
}:
let
  apksigner = pkgs.writeShellScriptBin "apksigner" ''
    unset JAVA_TOOL_OPTIONS _JAVA_OPTIONS JDK_JAVA_OPTIONS JAVA_OPTIONS JAVA_OPTS CLASSPATH
    exec ${jdk}/bin/java -Xmx1024M -jar ${sdkRoot}/build-tools/${buildTools}/lib/apksigner.jar "$@"
  '';
  program = pkgs.writeText "aguatv-sign-player.py" (
    builtins.replaceStrings
      [ "@apksigner@" "@aapt2@" ]
      [ "${apksigner}/bin/apksigner" "${sdkRoot}/build-tools/${buildTools}/aapt2" ]
      (builtins.readFile ./apk-signer.py)
  );
in
pkgs.runCommand "aguatv-apk-signer-1" { meta.mainProgram = "aguatv-sign-player"; } ''
  mkdir -p $out/bin $out/lib
  cp ${program} $out/lib/aguatv-sign-player.py
  cat > $out/bin/aguatv-sign-player <<EOF
  #!${pkgs.runtimeShell}
  exec ${pkgs.python3}/bin/python3 -I $out/lib/aguatv-sign-player.py "\$@"
  EOF
  chmod +x $out/bin/aguatv-sign-player
  ln -s ${apksigner}/bin/apksigner $out/bin/apksigner
''
