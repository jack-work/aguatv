import argparse
import contextlib
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import shutil
import stat
import subprocess
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("signer", os.environ["SIGNER_MODULE"])
signer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(signer)


class CredentialMetadataTests(unittest.TestCase):
    release = "test-release"
    directory = "/run/credentials/aguatv-sign-player@test-release.service"

    def check(self, changes=None, directory=None, release=None, readonly=True):
        entries = {
            path: SimpleNamespace(st_mode=stat.S_IFDIR | 0o755, st_uid=0, st_gid=0)
            for path in ["/", "/run", "/run/credentials", self.directory]
        }
        entries[self.directory].st_mode = stat.S_IFDIR | 0o550
        for index, name in enumerate(signer.CREDENTIALS.values()):
            entries[name] = SimpleNamespace(st_mode=stat.S_IFREG | 0o440, st_uid=0, st_gid=0, st_dev=1, st_ino=index)
        for path, fields in (changes or {}).items():
            for field, value in fields.items():
                setattr(entries[path], field, value)

        def metadata(path, follow_symlinks=False):
            self.assertFalse(follow_symlinks)
            key = Path(path).name if Path(path).name in signer.CREDENTIALS.values() else str(path)
            return entries[key]

        flags = SimpleNamespace(f_flag=os.ST_RDONLY if readonly else 0)
        with patch.object(signer.os, "stat", side_effect=metadata), patch.object(signer.os, "statvfs", return_value=flags):
            return signer.credential_paths(directory or self.directory, release or self.release)

    def test_exact_readonly_systemd_shape_is_accepted_without_reading_credentials(self):
        paths = self.check()
        self.assertEqual(set(paths.values()), {self.directory + "/" + name for name in signer.CREDENTIALS.values()})

    def test_group_readable_files_outside_the_matching_unit_namespace_are_refused(self):
        for directory, release in [
            ("/tmp/credentials", self.release),
            ("/run/credentials/other.service", self.release),
            (self.directory, "another-release"),
            (self.directory, "../escape"),
        ]:
            with self.subTest(directory=directory, release=release), self.assertRaisesRegex(signer.SigningError, "private"):
                self.check(directory=directory, release=release)

    def test_changed_file_ownership_modes_and_types_are_refused(self):
        for field, value in [
            ("st_uid", 1234), ("st_gid", 1234),
            ("st_mode", stat.S_IFREG | 0o444), ("st_mode", stat.S_IFREG | 0o460),
            ("st_mode", stat.S_IFREG | 0o640), ("st_mode", stat.S_IFREG | 0o450),
            ("st_mode", stat.S_IFLNK | 0o440), ("st_mode", stat.S_IFIFO | 0o440),
        ]:
            with self.subTest(field=field, value=value), self.assertRaisesRegex(signer.SigningError, "private"):
                self.check({"keystore": {field: value}})

    def test_changed_parent_ownership_modes_and_types_are_refused(self):
        for field, value in [
            ("st_uid", 1234), ("st_gid", 1234),
            ("st_mode", stat.S_IFDIR | 0o555), ("st_mode", stat.S_IFDIR | 0o570),
            ("st_mode", stat.S_IFLNK | 0o550),
        ]:
            with self.subTest(field=field, value=value), self.assertRaisesRegex(signer.SigningError, "private"):
                self.check({self.directory: {field: value}})
        for parent in ["/", "/run", "/run/credentials"]:
            for fields in [{"st_uid": 1234}, {"st_mode": stat.S_IFDIR | 0o775}, {"st_mode": stat.S_IFLNK | 0o755}]:
                with self.subTest(parent=parent, fields=fields), self.assertRaisesRegex(signer.SigningError, "private"):
                    self.check({parent: fields})

    def test_writable_filesystem_and_shared_credential_inodes_are_refused(self):
        with self.assertRaisesRegex(signer.SigningError, "private"):
            self.check(readonly=False)
        with self.assertRaisesRegex(signer.SigningError, "distinct"):
            self.check({"key-password": {"st_ino": 1}})


class SignerTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.fixture = tempfile.TemporaryDirectory()
        cls.credentials = Path(cls.fixture.name) / "credentials"
        cls.credentials.mkdir(mode=0o700)
        for name in ("store-password", "key-password"):
            path = cls.credentials / name
            path.write_text("ephemeral-fixture-password\n")
            path.chmod(0o600)
        subprocess.run(["keytool", "-genkeypair", "-alias", "home", "-keyalg", "RSA", "-keysize", "2048",
                        "-dname", "CN=Ephemeral signer test", "-validity", "1", "-storetype", "PKCS12",
                        "-keystore", str(cls.credentials / "keystore"),
                        "-storepass:file", str(cls.credentials / "store-password"),
                        "-keypass:file", str(cls.credentials / "key-password")],
                       check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        (cls.credentials / "keystore").chmod(0o600)
        certificate = Path(cls.fixture.name) / "public-certificate.der"
        subprocess.run(["keytool", "-exportcert", "-alias", "home", "-keystore", str(cls.credentials / "keystore"),
                        "-storepass:file", str(cls.credentials / "store-password"), "-file", str(certificate)],
                       check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        cls.fingerprint = hashlib.sha256(certificate.read_bytes()).hexdigest()

    @classmethod
    def tearDownClass(cls):
        cls.fixture.cleanup()

    def setUp(self):
        self.workspace = tempfile.TemporaryDirectory()
        self.addCleanup(self.workspace.cleanup)
        self.root = Path(self.workspace.name)
        self.inbox = self.root / "inbox"
        self.inbox.mkdir()
        self.output = self.root / "out"
        self.output.mkdir(mode=0o700)
        self.release = "test-1"
        self.staged = self.inbox / self.release
        self.staged.mkdir()
        shutil.copyfile(os.environ["UNSIGNED_APK"], self.staged / "unsigned.apk")
        self.receipt = {"schemaVersion": 1, "releaseId": self.release, "sourceRevision": "a" * 40,
                        "unsignedSha256": signer.checksum(self.staged / "unsigned.apk")}
        self.write_receipt()
        self.config = {"inboxDirectory": str(self.inbox), "outputDirectory": str(self.output),
                       "expectedCertificateSha256": self.fingerprint, **signer.CREDENTIALS}

    def write_receipt(self):
        (self.staged / "receipt.json").write_text(json.dumps(self.receipt))

    @contextlib.contextmanager
    def fixture_approval(self):
        with patch.object(signer, "trusted_stat"):
            yield

    def sign(self):
        with self.fixture_approval():
            return signer.sign_release(self.config, self.release, str(self.credentials))

    def test_real_ephemeral_key_signs_only_fixed_package_and_publishes_exact_artifacts(self):
        manifest = self.sign()
        published = self.output / self.release
        self.assertEqual({p.name for p in published.iterdir()}, {"aguatv-player.apk", "manifest.json"})
        self.assertEqual(manifest, json.loads((published / "manifest.json").read_text()))
        self.assertEqual(manifest["signedSha256"], signer.checksum(published / "aguatv-player.apk"))
        self.assertEqual(manifest["unsignedSha256"], self.receipt["unsignedSha256"])
        self.assertEqual(manifest["certSha256"], self.fingerprint)
        self.assertEqual(manifest["package"], "tv.aguatv.player")
        self.assertEqual(manifest["versionCode"], 1)
        self.assertEqual(manifest["versionName"], "0.1.0")
        self.assertFalse((self.output / (".tmp-" + self.release)).exists())
        serialized = json.dumps(manifest)
        self.assertNotIn("ephemeral-fixture-password", serialized)
        self.assertNotIn(str(self.credentials), serialized)
        self.assertNotIn(str(self.inbox), serialized)
        self.assertEqual(signer.checksum(self.staged / "unsigned.apk"), self.receipt["unsignedSha256"])
        with self.assertRaisesRegex(signer.SigningError, "already exists"):
            self.sign()
        self.assertEqual(manifest["signedSha256"], signer.checksum(published / "aguatv-player.apk"))

    def test_certificate_mismatch_never_publishes(self):
        self.config["expectedCertificateSha256"] = "0" * 64
        with self.assertRaisesRegex(signer.SigningError, "pinned certificate"):
            self.sign()
        self.assertFalse((self.output / self.release).exists())
        self.assertFalse((self.output / (".tmp-" + self.release)).exists())

    def test_unsigned_hash_mismatch_fails_before_any_tool_execution(self):
        self.receipt["unsignedSha256"] = "0" * 64
        self.write_receipt()
        with patch.object(signer, "run_tool", side_effect=AssertionError("tool must not run")):
            with self.assertRaisesRegex(signer.SigningError, "checksum"):
                self.sign()
        self.assertFalse((self.output / self.release).exists())

    def test_wrong_package_fails_before_signing(self):
        with patch.object(signer, "run_tool", return_value="package: name='tv.aguatv' versionCode='1' versionName='0.1.0'") as tool:
            with self.assertRaisesRegex(signer.SigningError, "tv.aguatv.player"):
                self.sign()
        self.assertEqual(tool.call_count, 1)
        self.assertFalse((self.output / self.release).exists())

    def test_approval_rejects_nonroot_writable_and_nonregular_objects(self):
        signer.trusted_stat(SimpleNamespace(st_uid=0, st_mode=stat.S_IFREG | 0o644), False)
        signer.trusted_stat(SimpleNamespace(st_uid=0, st_mode=stat.S_IFDIR | 0o755), True)
        for uid, mode, directory in ((1, stat.S_IFREG | 0o644, False), (0, stat.S_IFREG | 0o664, False),
                                     (0, stat.S_IFDIR | 0o777, True), (0, stat.S_IFLNK | 0o777, False),
                                     (0, stat.S_IFIFO | 0o600, False)):
            with self.subTest(uid=uid, mode=mode):
                with self.assertRaises(signer.SigningError):
                    signer.trusted_stat(SimpleNamespace(st_uid=uid, st_mode=mode), directory)

    def test_approval_walk_rejects_symlink_file_and_directory(self):
        real = self.staged / "receipt.json"
        alias = self.staged / "alias.json"
        alias.symlink_to(real)
        directory_alias = self.inbox / "alias"
        directory_alias.symlink_to(self.staged, target_is_directory=True)
        with self.fixture_approval():
            for path in (alias, directory_alias / "receipt.json"):
                with self.assertRaises(OSError):
                    signer.open_approved(path)

    def test_release_id_is_checked_before_any_path_access(self):
        for value in ("../escape", ".", "..", "A", "x/y", "a" * 65, "é", "-x", ""):
            with self.subTest(value=value), patch.object(signer, "credential_paths", side_effect=AssertionError("path access")):
                with self.assertRaises(argparse.ArgumentTypeError):
                    signer.sign_release(self.config, value, str(self.credentials))
        self.assertEqual(signer.release_id("v0.1.0-native_1"), "v0.1.0-native_1")

    def test_receipt_requires_exact_schema_release_revision_and_hash(self):
        for field, value in (("schemaVersion", True), ("releaseId", "other"), ("sourceRevision", "not-a-revision"),
                             ("unsignedSha256", "not-a-hash"), ("command", "anything")):
            with self.subTest(field=field):
                original = dict(self.receipt)
                self.receipt[field] = value
                self.write_receipt()
                with self.assertRaisesRegex(signer.SigningError, "receipt"):
                    self.sign()
                self.receipt = original

    def test_duplicate_json_fields_are_rejected(self):
        path = self.staged / "receipt.json"
        path.write_text('{"schemaVersion":1,"schemaVersion":1}')
        with self.assertRaisesRegex(signer.SigningError, "duplicate"):
            self.sign()

    def test_credential_paths_require_private_distinct_regular_files(self):
        paths = signer.credential_paths(str(self.credentials))
        self.assertNotEqual(paths["storePasswordFile"], paths["keyPasswordFile"])
        with tempfile.TemporaryDirectory() as directory:
            directory = Path(directory)
            for name in signer.CREDENTIALS.values():
                (directory / name).write_text("test-only\n")
                (directory / name).chmod(0o600)
            (directory / "key-password").unlink()
            os.link(directory / "store-password", directory / "key-password")
            with self.assertRaisesRegex(signer.SigningError, "distinct"):
                signer.credential_paths(str(directory))
            (directory / "key-password").unlink()
            (directory / "key-password").symlink_to(directory / "store-password")
            with self.assertRaisesRegex(signer.SigningError, "regular"):
                signer.credential_paths(str(directory))

    def test_publish_does_not_replace_even_an_empty_release_directory(self):
        temporary = self.output / ".tmp-test"
        temporary.mkdir()
        target = self.output / "existing"
        target.mkdir()
        fd = os.open(self.output, os.O_RDONLY | os.O_DIRECTORY)
        try:
            with self.assertRaisesRegex(signer.SigningError, "never replaced"):
                signer.publish(fd, temporary.name, target.name)
        finally:
            os.close(fd)
        self.assertTrue(temporary.is_dir())
        self.assertTrue(target.is_dir())

    def test_private_systemd_output_backing_symlink_is_supported(self):
        link = self.root / "state-link"
        link.symlink_to(self.output, target_is_directory=True)
        self.config["outputDirectory"] = str(link)
        self.receipt["unsignedSha256"] = "0" * 64
        self.write_receipt()
        with self.assertRaisesRegex(signer.SigningError, "checksum"):
            self.sign()

    def test_child_environment_drops_ambient_java_and_password_variables(self):
        with patch.dict(os.environ, {"JAVA_TOOL_OPTIONS": "untrusted", "KEYPASS": "not-a-real-secret"}):
            with patch.object(subprocess, "run", return_value=SimpleNamespace(stdout="ok")) as run:
                self.assertEqual(signer.run_tool(["fixed-tool", "verify"], self.root, "test"), "ok")
        environment = run.call_args.kwargs["env"]
        self.assertNotIn("JAVA_TOOL_OPTIONS", environment)
        self.assertNotIn("KEYPASS", environment)
        self.assertEqual(run.call_args.kwargs["stdin"], subprocess.DEVNULL)
        self.assertEqual(run.call_args.kwargs["stderr"], subprocess.DEVNULL)

    def test_config_requires_fixed_credential_basenames(self):
        path = self.root / "config.json"
        path.write_text(json.dumps(self.config))
        with self.fixture_approval():
            self.assertEqual(signer.load_config(path), self.config)
            self.config["runtimeKeystoreFile"] = "/arbitrary/key"
            path.write_text(json.dumps(self.config))
            with self.assertRaisesRegex(signer.SigningError, "credential names"):
                signer.load_config(path)

    def test_packaged_cli_help_and_invalid_id_do_not_open_runtime_configuration(self):
        binary = os.environ["SIGNER_BIN"]
        for option in ("--help", "--version"):
            result = subprocess.run([binary, option], capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
        result = subprocess.run([binary, "../escape"], capture_output=True, text=True,
                                env={"AGUATV_SIGNER_CONFIG": "/must-not-open"})
        self.assertEqual(result.returncode, 2)
        self.assertNotIn("filesystem", result.stderr)


if __name__ == "__main__":
    unittest.main(verbosity=2)
