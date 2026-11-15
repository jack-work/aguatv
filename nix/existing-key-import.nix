{ pkgs, jdk }:
let
  config = pkgs.writeText "aguatv-import-sops.yaml" ''
    creation_rules:
      - path_regex: secrets/aguatv-release-(keystore|store-password|key-password)\.enc\.json$
        age: age12edvyhzhyd4afl4veljtyt572ga430dk29unpszp2n2vn244zdzqa9zhu7,age1tn43d3js9al679lr67724rr98rvgkczqg602673pex76qesg5p6qw5h7al,age136h60ucfnz4w480dakvpx26k54l3j04yzpx5rmgdcqzw8nnn8q4qp86pwa
  '';
  program = pkgs.writeTextDir "ExistingKeyImport.java" (builtins.replaceStrings
    [ "@sops@" "@config@" ] [ "${pkgs.sops}/bin/sops" "${config}" ]
    (builtins.readFile ./ExistingKeyImport.java));
  classes = pkgs.runCommand "aguatv-existing-key-import-classes" { nativeBuildInputs = [ jdk ]; } ''
    mkdir -p $out
    javac -encoding UTF-8 -d $out ${program}/ExistingKeyImport.java
  '';
in
(pkgs.writeShellScriptBin "aguatv-existing-key-import" ''
  set +x +v
  set -euo pipefail
  ulimit -c 0
  unset JAVA_TOOL_OPTIONS _JAVA_OPTIONS JDK_JAVA_OPTIONS JAVA_OPTIONS JAVA_OPTS CLASSPATH LD_PRELOAD LD_LIBRARY_PATH
  exec ${jdk}/bin/java -Xmx256m -XX:+DisableAttachMechanism -XX:-HeapDumpOnOutOfMemoryError -XX:ErrorFile=/dev/null -cp ${classes} ExistingKeyImport "$@"
'').overrideAttrs (_: { passthru = { inherit classes config; }; })
