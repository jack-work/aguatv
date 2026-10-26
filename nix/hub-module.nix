self:
{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.services.aguatv-hub;
  own = self.packages.${pkgs.stdenv.hostPlatform.system};
in
{
  options.services.aguatv-hub = {
    enable = lib.mkEnableOption "the aguatv hub: a person's screens, casting and the web app";

    package = lib.mkOption {
      type = lib.types.package;
      default = own.hub;
      defaultText = lib.literalExpression "aguatv.packages.\${system}.hub";
    };

    site = lib.mkOption {
      type = lib.types.package;
      default = own.web;
      defaultText = lib.literalExpression "aguatv.packages.\${system}.web";
      description = "The web app the hub serves, with its .csp hash files.";
    };

    filesOrigin = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "https://files.example.com";
      description = "The only origin a cast may name. Screens fetch it with their own login.";
    };

    listen = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "127.0.0.1:8740";
      description = ''
        TCP address to listen on. Null means a socket unit hands the hub its
        listener, which is how it should run behind a proxy that sets
        Remote-User: nothing else can then reach it and forge the header.
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    systemd.services.aguatv-hub = {
      description = "aguatv hub";
      wantedBy = lib.mkIf (cfg.listen != null) [ "multi-user.target" ];
      serviceConfig = {
        ExecStart = lib.escapeShellArgs (
          [
            "${cfg.package}/bin/aguatv-hub"
            "-site"
            "${cfg.site}"
            "-state"
            "/var/lib/aguatv-hub"
          ]
          ++ lib.optionals (cfg.filesOrigin != null) [
            "-files"
            cfg.filesOrigin
          ]
          ++ lib.optionals (cfg.listen != null) [
            "-listen"
            cfg.listen
          ]
        );
        DynamicUser = true;
        StateDirectory = "aguatv-hub";
        StateDirectoryMode = "0700";
        UMask = "0077";
        Restart = "on-failure";
        RestartSec = 5;
        PrivateNetwork = cfg.listen == null;
        RestrictAddressFamilies = if cfg.listen == null then [ "AF_UNIX" ] else [ "AF_INET" "AF_INET6" "AF_UNIX" ];
        NoNewPrivileges = true;
        PrivateTmp = true;
        PrivateDevices = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        ProtectKernelTunables = true;
        ProtectKernelModules = true;
        ProtectKernelLogs = true;
        ProtectControlGroups = true;
        ProtectClock = true;
        ProtectHostname = true;
        ProtectProc = "invisible";
        ProcSubset = "pid";
        RestrictNamespaces = true;
        RestrictRealtime = true;
        RestrictSUIDSGID = true;
        LockPersonality = true;
        MemoryDenyWriteExecute = true;
        SystemCallFilter = [ "@system-service" "~@privileged" "~@resources" ];
        SystemCallArchitectures = "native";
        CapabilityBoundingSet = "";
        MemoryMax = "128M";
        CPUQuota = "50%";
      };
    };
  };
}
