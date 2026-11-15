{ pkgs, apk-signer }:
let
  dummy = pkgs.writeText "public-credential-fixture" "not-a-key-or-password\n";
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
in
pkgs.testers.runNixOSTest {
  name = "aguatv-signer-credentials";
  nodes.machine = { ... }: {
    systemd.services."aguatv-sign-player@".serviceConfig = {
      Type = "oneshot";
      DynamicUser = true;
      User = "aguatv-signer";
      ExecStart = "${pkgs.python3}/bin/python3 -I ${check}";
      LoadCredential = [ "keystore:${dummy}" "store-password:${dummy}" "key-password:${dummy}" ];
      PrivateNetwork = true;
      PrivateTmp = true;
      ProtectSystem = "strict";
      NoNewPrivileges = true;
      CapabilityBoundingSet = "";
    };
  };
  testScript = ''
    machine.succeed("systemctl start aguatv-sign-player@probe.service")
    machine.succeed("test $(systemctl show -p ExecMainStatus --value aguatv-sign-player@probe.service) = 0")
  '';
}
