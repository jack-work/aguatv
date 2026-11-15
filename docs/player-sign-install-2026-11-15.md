# Sign and update the TV receiver

Gluck runs these commands on **gluck**, in Bash. Agents have not signed or installed this release. The imported existing key is already wired to Spain's manual signing unit. No keystore password belongs in these commands or in chat. If a password prompt appears, type it yourself.

Source: `5b26945c23a3b999704ff7f6bd75f081a5bccb90`, containing receiver repair `f978175` as `ee67d7a`. Release ID: `aguatv-player-0.1.0-5b26945`.

Run each block only after the preceding block succeeds. Stop on any error. Do not rerun against an existing release directory or delete it to bypass a refusal.

## 1. Build and check the unsigned player

```bash
set -euo pipefail
R=aguatv-player-0.1.0-5b26945
REV=5b26945c23a3b999704ff7f6bd75f081a5bccb90
HASH=3fccbccff64b67c28b8b2b046d08e1cac5f0808a629f7fa2a4248eaced34d19d
BUILD=$(nix build --no-link --print-out-paths "git+ssh://git@spain/var/lib/keel/jack/aguatv.git?rev=$REV#player")
APK="$BUILD/aguatv-player-unsigned.apk"
printf '%s  %s\n' "$HASH" "$APK" | sha256sum -c -
```

## 2. Approve the exact APK and invoke the new unit

This creates a new root-owned approval inbox, transfers only the unsigned APK and receipt, then signs using the already imported credentials. It does not upload or install anything on the TV. Spain's wheel sudo is configured without a password, so `sudo -n` should succeed; otherwise stop.

```bash
ssh spain 'set -eu
I=/var/lib/aguatv-release/inbox/aguatv-player-0.1.0-5b26945
sudo -n install -d -o root -g root -m 0755 /var/lib/aguatv-release /var/lib/aguatv-release/inbox
sudo -n test ! -e "$I"
sudo -n mkdir -m 0755 "$I"'

ssh spain 'sudo -n tee /var/lib/aguatv-release/inbox/aguatv-player-0.1.0-5b26945/unsigned.apk >/dev/null' < "$APK"

printf '{"schemaVersion":1,"releaseId":"%s","sourceRevision":"%s","unsignedSha256":"%s"}\n' "$R" "$REV" "$HASH" |
  ssh spain 'sudo -n tee /var/lib/aguatv-release/inbox/aguatv-player-0.1.0-5b26945/receipt.json >/dev/null'

ssh spain 'set -eu
R=aguatv-player-0.1.0-5b26945
I=/var/lib/aguatv-release/inbox/$R
sudo -n chmod 0644 "$I/unsigned.apk" "$I/receipt.json"
printf "%s  %s\n" 3fccbccff64b67c28b8b2b046d08e1cac5f0808a629f7fa2a4248eaced34d19d "$I/unsigned.apk" | sudo -n sha256sum -c -
sudo -n systemctl start "aguatv-sign-player@$R.service"
sudo -n cat "/var/lib/private/aguatv-release/out/$R/manifest.json"'
```

## 3. Retrieve and verify the signed APK

These SSH commands deliberately have no `-t`: a terminal can corrupt binary output.

```bash
D=$(mktemp -d "$HOME/aguatv-player-release.XXXXXX")
ssh spain 'sudo -n cat /var/lib/private/aguatv-release/out/aguatv-player-0.1.0-5b26945/manifest.json' > "$D/manifest.json"
ssh spain 'sudo -n cat /var/lib/private/aguatv-release/out/aguatv-player-0.1.0-5b26945/aguatv-player.apk' > "$D/aguatv-player.apk"
python3 - "$D" <<'PY'
import hashlib
import json
import pathlib
import sys
p = pathlib.Path(sys.argv[1])
m = json.loads((p / 'manifest.json').read_text())
expected = {
    'releaseId': 'aguatv-player-0.1.0-5b26945',
    'sourceRevision': '5b26945c23a3b999704ff7f6bd75f081a5bccb90',
    'unsignedSha256': '3fccbccff64b67c28b8b2b046d08e1cac5f0808a629f7fa2a4248eaced34d19d',
    'certSha256': '642be2dcf5893a7e7024d32731f90aebf5f329c938bd48fdbf79ce1c52d20d44',
    'package': 'tv.aguatv.player',
}
for key, value in expected.items():
    if m.get(key) != value:
        raise SystemExit(f'STOP: manifest {key} mismatch')
if hashlib.sha256((p / 'aguatv-player.apk').read_bytes()).hexdigest() != m.get('signedSha256'):
    raise SystemExit('STOP: signed APK hash mismatch')
print('PASS: release identity, certificate pin and signed APK hash match')
PY
```

## 4. Update in place, then reopen once

With the intended TV already connected and authorized in ADB:

```bash
adb install -r "$D/aguatv-player.apk"
```

If multiple devices are connected, select the TV explicitly with `adb -s SERIAL install -r "$D/aguatv-player.apk"`, using its actual serial from `adb devices`. If Android reports a signing mismatch, **stop**. Never uninstall, run `pm clear`, or generate a new key to get past it. `install -r` retains the receiver's app data and pairing.

After installation, close and reopen the TV receiver once so it loads the current web player. Then try Controls and D-pad navigation, and report what happens. Original-quality subtitles have worked on the phone; native-HLS subtitles and actual TV/Safari behavior remain unproven.
