{ pkgs, jdk, helper }:
pkgs.runCommand "aguatv-existing-key-import-tests" {
  nativeBuildInputs = [ jdk pkgs.python3 pkgs.coreutils ];
} ''
  set -eu
  export HOME=$TMPDIR/home
  mkdir -m 700 "$HOME" fixture
  printf ' fixture store \n' > fixture/store-password
  printf ' fixture key \n' > fixture/key-password
  printf 'fixture-p12\n' > fixture/p12-password
  chmod 600 fixture/*-password
  keytool -genkeypair -alias home -keyalg RSA -keysize 2048 -dname 'CN=Ephemeral import fixture' -validity 1 \
    -storetype JKS -keystore fixture/fixture.jks -storepass:file fixture/store-password -keypass:file fixture/key-password > fixture-keytool.log 2>&1 || { cat fixture-keytool.log; exit 1; }
  keytool -genkeypair -alias home -keyalg RSA -keysize 2048 -dname 'CN=Ephemeral PKCS12 fixture' -validity 1 \
    -storetype PKCS12 -keystore fixture/fixture.p12 -storepass:file fixture/p12-password -keypass:file fixture/p12-password > fixture-keytool.log 2>&1 || { cat fixture-keytool.log; exit 1; }
  chmod 600 fixture/fixture.*
  cat > fixture/stub.py <<'PY'
  import json
  import os
  from pathlib import Path
  import sys
  name = Path(sys.argv[-1]).name
  expected = {
      "aguatv-release-keystore.enc.json": Path(__file__).with_name("fixture.jks").read_bytes(),
      "aguatv-release-store-password.enc.json": b" fixture store ",
      "aguatv-release-key-password.enc.json": b" fixture key ",
  }
  assert sys.stdin.buffer.read() == expected[name]
  assert "UNRELATED_PASSWORD" not in os.environ
  assert "JAVA_TOOL_OPTIONS" not in os.environ
  print(json.dumps({"data": "ENC[synthetic-fixture]", "sops": {}}))
  PY
  cat > fixture/fail.py <<'PY'
  import sys
  sys.stdin.buffer.read()
  if sys.argv[-1].endswith("key-password.enc.json"):
      print("DO_NOT_LOG_SYNTHETIC_STDERR", file=sys.stderr)
      raise SystemExit(7)
  print('{"data":"ENC[synthetic-fixture]","sops":{}}')
  PY
  cp ${./ExistingKeyImportTest.java} ExistingKeyImportTest.java
  mkdir test-classes
  javac -encoding UTF-8 -cp ${helper.classes} -d test-classes ExistingKeyImportTest.java
  UNRELATED_PASSWORD=DO_NOT_PROPAGATE java -cp ${helper.classes}:test-classes ExistingKeyImportTest \
    "$PWD/fixture/fixture.jks" "$PWD/fixture" ${pkgs.python3}/bin/python3 "$PWD/fixture/stub.py" "$PWD/fixture/fail.py" ${helper.config} ${pkgs.sops}/bin/sops > tests.log 2>&1 || { cat tests.log; exit 1; }
  ! grep -q DO_NOT_LOG_SYNTHETIC_STDERR tests.log
  python3 - "$PWD/fixture/real-sops" <<'PY'
  import json
  from pathlib import Path
  import sys
  files = list(Path(sys.argv[1]).glob("*.enc.json"))
  assert len(files) == 3
  for path in files:
      value = json.loads(path.read_text())
      assert value["data"].startswith("ENC[AES256_GCM,")
      assert len(value["sops"]["age"]) == 3
  print("PASS real SOPS binary/json encryption to the three existing public recipients")
  PY
  python3 - ${jdk}/bin/java ${helper.classes} "$PWD/fixture" <<'PY'
  import os
  from pathlib import Path
  import pty
  import select
  import shutil
  import subprocess
  import sys
  import time
  java, classes, fixture = sys.argv[1:]
  home = Path(fixture) / "console-home"
  source = home / ".local/share/aguatv/release.keystore"
  source.parent.mkdir(parents=True)
  shutil.copyfile(Path(fixture) / "fixture.jks", source)
  source.chmod(0o600)
  command = [java, "-Duser.home=" + str(home), "-cp", classes, "ExistingKeyImport"]
  no_console = subprocess.run(command, input=b"not-a-password\n", stdout=subprocess.PIPE, stderr=subprocess.PIPE)
  assert no_console.returncode == 2 and no_console.stderr.strip() == b"INTERACTIVE_CONSOLE_REQUIRED"
  pid, master = pty.fork()
  if pid == 0:
      os.execv(command[0], command)
  transcript = b""
  sent_store = sent_key = False
  deadline = time.monotonic() + 30
  while time.monotonic() < deadline:
      ready, _, _ = select.select([master], [], [], max(0, deadline - time.monotonic()))
      if not ready:
          os.kill(pid, 9)
          raise AssertionError("synthetic console test timed out")
      try:
          chunk = os.read(master, 4096)
      except OSError:
          break
      if not chunk:
          break
      transcript += chunk
      if b"STORE password: " in transcript and not sent_store:
          os.write(master, b" fixture store \n")
          sent_store = True
      if b"alias home): " in transcript and not sent_key:
          os.write(master, b" fixture key \n")
          sent_key = True
  _, status = os.waitpid(pid, 0)
  os.close(master)
  assert sent_store and sent_key and os.waitstatus_to_exitcode(status) == 1
  assert b"CERTIFICATE_PIN_MISMATCH" in transcript
  assert b" fixture store " not in transcript and b" fixture key " not in transcript
  assert not (home / ".local/state/aguatv-key-import/validated").exists()
  assert source.read_bytes() == (Path(fixture) / "fixture.jks").read_bytes()
  print("PASS hidden real Console prompts, no TTY refusal, fixed pin mismatch and original preservation")
  PY
  cat tests.log > $out
''
