{ pkgs, self }:
pkgs.testers.runNixOSTest {
  name = "aguatv-hub-module";
  nodes.machine = {
    imports = [ self.nixosModules.hub ];
    services.aguatv-hub = {
      enable = true;
      filesOrigin = "https://files.example.com";
    };
    systemd.sockets.aguatv-hub = {
      wantedBy = [ "sockets.target" ];
      socketConfig.ListenStream = "/run/aguatv-hub.sock";
    };
  };
  testScript = ''
    machine.wait_for_unit("sockets.target")
    curl = "curl -sf --unix-socket /run/aguatv-hub.sock -H 'Remote-User: jack' "
    machine.fail("curl -sf --unix-socket /run/aguatv-hub.sock http://tv/api/me")
    machine.succeed(curl + "http://tv/ | grep -c aguatv")
    csp = machine.succeed(curl + "-D- -o /dev/null http://tv/ | grep -i content-security-policy")
    assert "media-src 'self' https://files.example.com" in csp, csp
    assert "sha256-" in csp, csp
    head = machine.succeed(curl + "-D- -o /dev/null -H 'Content-Type: application/json' -d '{\"name\":\"den\",\"remote\":true}' http://tv/api/me")
    cookie = [l.split(":", 1)[1].split(";")[0].strip() for l in head.splitlines() if l.lower().startswith("set-cookie:")][0]
    me = curl + f"-H 'Cookie: {cookie}' http://tv/api/me"
    assert '"remote":true' in machine.succeed(me)
    machine.succeed("systemctl restart aguatv-hub.service")
    assert '"name":"den"' in machine.succeed(me)
    machine.succeed("test \"$(stat -c %a /var/lib/private/aguatv-hub)\" = 700")
  '';
}
