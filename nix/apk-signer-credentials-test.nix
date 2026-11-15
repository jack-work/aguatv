{ pkgs, apk-signer, player, jdk }:
let
  fixture = pkgs.writeText "prepare-public-signer-fixture.py" ''
    import hashlib
    import json
    import os
    from pathlib import Path
    import shutil
    import subprocess
    root = Path("/run/signer-fixture")
    root.mkdir(mode=0o700)
    password = root / "fixture-password"
    password.write_text("ephemeral-test-password\n")
    password.chmod(0o600)
    key = root / "keystore"
    subprocess.run(["${jdk}/bin/keytool", "-genkeypair", "-alias", "home", "-keyalg", "RSA", "-keysize", "2048",
                    "-dname", "CN=Ephemeral VM fixture", "-validity", "1", "-storetype", "PKCS12", "-keystore", str(key),
                    "-storepass:file", str(password), "-keypass:file", str(password)],
                   check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    certificate = root / "public-certificate.der"
    subprocess.run(["${jdk}/bin/keytool", "-exportcert", "-alias", "home", "-keystore", str(key),
                    "-storepass:file", str(password), "-file", str(certificate)],
                   check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    inbox = Path("/var/lib/signer-fixture/inbox/signed-probe")
    inbox.mkdir(mode=0o755, parents=True)
    apk = inbox / "unsigned.apk"
    shutil.copyfile("${player}/aguatv-player-unsigned.apk", apk)
    apk.chmod(0o644)
    (inbox / "receipt.json").write_text(json.dumps({"schemaVersion": 1, "releaseId": "signed-probe",
        "sourceRevision": "0" * 40, "unsignedSha256": hashlib.sha256(apk.read_bytes()).hexdigest()}))
    (inbox / "receipt.json").chmod(0o644)
    Path("/etc/signer-fixture.json").write_text(json.dumps({"inboxDirectory": str(inbox.parent),
        "outputDirectory": "/var/lib/signer-fixture/out", "expectedCertificateSha256": hashlib.sha256(certificate.read_bytes()).hexdigest(),
        "runtimeKeystoreFile": "keystore", "storePasswordFile": "store-password", "keyPasswordFile": "key-password"}))
  '';
  check = pkgs.writeText "credential-contract.py" ''
    import importlib.util
    import os
    spec = importlib.util.spec_from_file_location("signer", "${apk-signer}/lib/aguatv-sign-player.py")
    signer = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(signer)
    assert os.geteuid() != 0
    paths = signer.credential_paths(os.environ["CREDENTIALS_DIRECTORY"], "probe")
    assert len(paths) == 3
    print("systemd credential namespace contract accepted; no credential contents read")
  '';
  run = pkgs.writeShellScript "signer-fixture-run" ''
    if [ "$1" = probe ]; then
      exec ${pkgs.python3}/bin/python3 -I ${check}
    fi
    exec ${apk-signer}/bin/aguatv-sign-player "$1"
  '';
in
pkgs.testers.runNixOSTest {
  name = "aguatv-signer-credentials";
  nodes.machine = { ... }: {
    systemd.services."aguatv-sign-player@" = {
      environment.AGUATV_SIGNER_CONFIG = "/etc/signer-fixture.json";
      serviceConfig = {
        Type = "oneshot";
        DynamicUser = true;
        User = "aguatv-signer";
        ExecStart = "${run} %i";
        LoadCredential = [ "keystore:/run/signer-fixture/keystore" "store-password:/run/signer-fixture/fixture-password" "key-password:/run/signer-fixture/fixture-password" ];
        StateDirectory = "signer-fixture/out";
        StateDirectoryMode = "0700";
        UMask = "0077";
        UnsetEnvironment = [ "JAVA_TOOL_OPTIONS" "_JAVA_OPTIONS" "JDK_JAVA_OPTIONS" "JAVA_OPTIONS" "JAVA_OPTS" "CLASSPATH" ];
        PrivateNetwork = true;
        RestrictAddressFamilies = [ "AF_UNIX" ];
        IPAddressDeny = "any";
        PrivateTmp = true;
        PrivateDevices = true;
        ProtectSystem = "strict";
        ReadOnlyPaths = [ "/var/lib/signer-fixture/inbox" ];
        ProtectHome = true;
        ProtectProc = "invisible";
        NoNewPrivileges = true;
        CapabilityBoundingSet = "";
        ProtectKernelLogs = true;
        ProtectKernelModules = true;
        ProtectKernelTunables = true;
        ProtectControlGroups = true;
        LockPersonality = true;
        RestrictNamespaces = true;
        RestrictSUIDSGID = true;
        LimitCORE = 0;
        TimeoutStartSec = "5min";
      };
    };
  };
  testScript = ''
    import json
    machine.succeed("${pkgs.python3}/bin/python3 ${fixture}")
    machine.succeed("systemctl start aguatv-sign-player@probe.service")
    machine.succeed("test $(systemctl show -p ExecMainStatus --value aguatv-sign-player@probe.service) = 0")
    machine.succeed("systemctl start aguatv-sign-player@signed-probe.service")
    manifest = json.loads(machine.succeed("cat /var/lib/private/signer-fixture/out/signed-probe/manifest.json"))
    assert manifest["releaseId"] == "signed-probe" and manifest["package"] == "tv.aguatv.player", manifest
  '';
}
