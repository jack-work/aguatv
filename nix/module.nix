{ lib, config, ... }:
let
  inherit (lib) mkOption types;
  value = types.oneOf [
    types.str
    types.int
    types.float
    types.bool
  ];
  package = types.submodule {
    options = {
      state = mkOption {
        type = types.enum [
          "enabled"
          "disabled"
          "absent"
          "unmanaged"
        ];
        default = "unmanaged";
      };
      protected = mkOption {
        type = types.bool;
        default = false;
      };
      pinned = mkOption {
        type = types.bool;
        default = false;
      };
      group = mkOption {
        type = types.nullOr types.str;
        default = null;
      };
      note = mkOption {
        type = types.str;
        default = "";
      };
    };
  };
  render = v: if builtins.isBool v then (if v then "1" else "0") else toString v;
  pinnedViolations = lib.filterAttrs (
    _: p: p.pinned && (p.state == "disabled" || p.state == "absent")
  ) config.packages;
in
{
  options = {
    name = mkOption { type = types.str; };
    address = mkOption {
      type = types.nullOr types.str;
      default = null;
    };
    serial = mkOption {
      type = types.nullOr types.str;
      default = null;
    };
    packages = mkOption {
      type = types.attrsOf package;
      default = { };
    };
    settings = mkOption {
      type = types.attrsOf (types.attrsOf value);
      default = { };
    };
    home.enable = mkOption {
      type = types.bool;
      default = true;
    };
    player.enable = mkOption {
      type = types.bool;
      default = false;
    };
    plan = mkOption {
      type = types.attrs;
      readOnly = true;
    };
  };

  config.plan =
    assert lib.assertMsg (pinnedViolations == { })
      "aguatv: pinned packages cannot be disabled or removed: ${lib.concatStringsSep ", " (lib.attrNames pinnedViolations)}. Set pinned = false beside the state if you mean it.";
    assert lib.assertMsg (
      config.address != null || config.serial != null
    ) "aguatv: ${config.name} needs an address (host:port) or a serial";
    {
      inherit (config)
        name
        address
        serial
        packages
        home
        player
        ;
      settings = lib.mapAttrs (_: lib.mapAttrs (_: render)) config.settings;
    };
}
