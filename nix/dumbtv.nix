{ dumbtv }:
{ lib, config, ... }:
let
  inherit (lib) mkOption types;
  cfg = config.dumbtv;
  dir = "${dumbtv}/data/fireos";

  parse =
    file:
    let
      lines = lib.splitString "\n" (builtins.readFile "${dir}/${file}");
      step =
        acc: raw:
        let
          line = lib.trim raw;
          header = builtins.match "# --- (.*) ---" line;
          more = builtins.match "[[:space:]]+#[[:space:]]*(.*)" raw;
          last = lib.last acc.entries;
          entry = builtins.match "([A-Za-z0-9_.]+)[[:space:]]*(#[[:space:]]*(.*))?" line;
        in
        if more != null && acc.entries != [ ] then
          acc
          // {
            entries = lib.init acc.entries ++ [ (last // { note = last.note + " " + lib.head more; }) ];
          }
        else if header != null then
          acc // { group = lib.head header; }
        else if entry != null then
          acc
          // {
            entries = acc.entries ++ [
              {
                name = lib.elemAt entry 0;
                note = lib.optionalString (lib.elemAt entry 2 != null) (lib.elemAt entry 2);
                group = acc.group;
              }
            ];
          }
        else
          acc;
    in
    (lib.foldl' step {
      group = null;
      entries = [ ];
    } lines).entries;

  layer =
    file: extra:
    lib.listToAttrs (
      map (e: {
        inherit (e) name;
        value = lib.mapAttrs (_: lib.mkDefault) (
          {
            note = e.note;
          }
          // lib.optionalAttrs (e.group != null) { group = e.group; }
          // extra
        );
      }) (parse file)
    );

  settingsLines = lib.filter (l: l != "" && !lib.hasPrefix "#" l) (
    map lib.trim (lib.splitString "\n" (builtins.readFile "${dir}/settings.txt"))
  );
  settings = lib.foldl' (
    acc: l:
    let
      parts = lib.filter (p: p != "") (lib.splitString " " l);
    in
    lib.recursiveUpdate acc {
      ${lib.elemAt parts 0}.${lib.elemAt parts 1} = lib.mkDefault (lib.concatStringsSep " " (lib.drop 2 parts));
    }
  ) { } settingsLines;
in
{
  options.dumbtv = {
    enable = mkOption {
      type = types.bool;
      default = true;
    };
    alexa = mkOption {
      type = types.bool;
      default = true;
    };
  };

  config = lib.mkIf cfg.enable {
    packages = lib.mkMerge [
      (layer "packages.txt" { state = "disabled"; })
      (layer "protected.txt" {
        state = "disabled";
        protected = true;
      })
      (layer "alexa.txt" {
        state = if cfg.alexa then "unmanaged" else "disabled";
        group = "alexa";
      })
      (layer "user-apps.txt" { group = "apps"; })
      (layer "never-disable.txt" {
        state = "enabled";
        pinned = true;
      })
    ];
    inherit settings;
  };
}
