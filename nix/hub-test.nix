{ pkgs, self }:
pkgs.testers.runNixOSTest {
  name = "aguatv-hub-module";
  nodes.machine =
    { lib, ... }:
    {
      imports = [ self.nixosModules.hub ];
      services.aguatv-hub = {
        enable = true;
        filesOrigin = "https://files.example.com";
      };
      systemd.sockets.aguatv-hub = {
        wantedBy = [ "sockets.target" ];
        socketConfig.ListenStream = "/run/aguatv-hub.sock";
      };
      specialisation.static.configuration = {
        users.users.tv = {
          isSystemUser = true;
          group = "tv";
        };
        users.groups.tv = { };
        services.aguatv-hub = {
          user = "tv";
          group = "tv";
        };
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

    with subtest("a static user takes over the dynamic user's screens"):
        machine.succeed("/run/current-system/specialisation/static/bin/switch-to-configuration test")
        assert '"name":"den"' in machine.succeed(me)
        d = machine.succeed("stat -c '%a %U:%G' /var/lib/aguatv-hub").strip()
        assert d == "700 tv:tv", d
        assert machine.succeed("stat -c '%a %U:%G' /var/lib/aguatv-hub/screens.json").strip() == "600 tv:tv"
        pid = machine.succeed("systemctl show -p MainPID --value aguatv-hub.service").strip()
        assert machine.succeed(f"ps -o user= -p {pid}").strip() == "tv"
        machine.fail(f"grep idmapped /proc/{pid}/mountinfo")
        machine.succeed(curl + "-D- -o /dev/null -H 'Content-Type: application/json' -d '{\"name\":\"study\"}' http://tv/api/me")
        assert machine.succeed("stat -c '%a %U:%G' /var/lib/aguatv-hub/screens.json").strip() == "600 tv:tv"
        machine.succeed("systemctl restart aguatv-hub.service")
        machine.succeed(curl + "http://tv/api/me")
        assert machine.succeed("stat -c '%a %U:%G' /var/lib/aguatv-hub /var/lib/aguatv-hub/screens.json").split() == ["700", "tv:tv", "600", "tv:tv"]
        machine.fail("test -e /var/lib/private/aguatv-hub")
  '';
}
