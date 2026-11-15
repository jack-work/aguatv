import argparse
import ctypes
import datetime
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import sys
import threading

APKSIGNER = "@apksigner@"
AAPT2 = "@aapt2@"
PACKAGE = "tv.aguatv.player"
CONFIG = "/etc/aguatv-signer/config.json"
RELEASE_ID = re.compile(r"[a-z0-9][a-z0-9._-]{0,63}\Z")
HEX256 = re.compile(r"[0-9a-f]{64}\Z")
REVISION = re.compile(r"[0-9a-f]{40}\Z")
MAX_APK = 512 * 1024 * 1024
CREDENTIALS = {"runtimeKeystoreFile": "keystore", "storePasswordFile": "store-password", "keyPasswordFile": "key-password"}


class SigningError(Exception):
    pass


def release_id(value):
    if not RELEASE_ID.fullmatch(value):
        raise argparse.ArgumentTypeError("release ID must be 1..64 lowercase letters, digits, dots, underscores or hyphens, starting with a letter or digit")
    return value


def trusted_stat(info, directory):
    kind = stat.S_ISDIR if directory else stat.S_ISREG
    if info.st_uid != 0 or info.st_mode & 0o022 or not kind(info.st_mode):
        raise SigningError("approval paths must be root-owned, non-writable by group/other and free of symlinks")


def open_approved(path, directory=False):
    path = Path(path)
    if not path.is_absolute() or ".." in path.parts:
        raise SigningError("approval path must be absolute")
    fd = os.open("/", os.O_RDONLY | os.O_DIRECTORY)
    try:
        trusted_stat(os.fstat(fd), True)
        for index, part in enumerate(path.parts[1:]):
            is_directory = directory or index < len(path.parts) - 2
            flags = os.O_RDONLY | os.O_NOFOLLOW | (os.O_DIRECTORY if is_directory else os.O_NONBLOCK)
            next_fd = os.open(part, flags, dir_fd=fd)
            os.close(fd)
            fd = next_fd
            trusted_stat(os.fstat(fd), is_directory)
        return fd
    except BaseException:
        os.close(fd)
        raise


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise SigningError("duplicate JSON field")
        result[key] = value
    return result


def read_json(fd):
    with os.fdopen(fd, "rb") as stream:
        raw = stream.read(16385)
    if len(raw) > 16384:
        raise SigningError("JSON exceeds 16 KiB")
    try:
        value = json.loads(raw, object_pairs_hook=unique_object)
    except (ValueError, UnicodeError):
        raise SigningError("invalid JSON") from None
    if not isinstance(value, dict):
        raise SigningError("JSON must be an object")
    return value


def load_config(path):
    fd = os.open(path, os.O_RDONLY | os.O_NONBLOCK)
    try:
        trusted_stat(os.fstat(fd), False)
    except BaseException:
        os.close(fd)
        raise
    config = read_json(fd)
    fields = {"inboxDirectory", "outputDirectory", "expectedCertificateSha256", *CREDENTIALS}
    if set(config) != fields:
        raise SigningError("configuration fields do not match the signer interface")
    for field in ("inboxDirectory", "outputDirectory"):
        if not isinstance(config[field], str) or not Path(config[field]).is_absolute() or ".." in Path(config[field]).parts:
            raise SigningError("configured artifact directories must be absolute")
    fingerprint = config["expectedCertificateSha256"]
    if not isinstance(fingerprint, str) or not HEX256.fullmatch(fingerprint.lower()):
        raise SigningError("expectedCertificateSha256 must be a 64-hex public certificate fingerprint")
    config["expectedCertificateSha256"] = fingerprint.lower()
    if any(config[field] != name for field, name in CREDENTIALS.items()):
        raise SigningError("credential names must be keystore, store-password and key-password")
    return config


def systemd_credentials(directory, release):
    if not release or not RELEASE_ID.fullmatch(release):
        return False
    expected = Path("/run/credentials") / ("aguatv-sign-player@" + release + ".service")
    if Path(directory) != expected:
        return False
    for parent in [expected, *expected.parents]:
        info = os.stat(parent, follow_symlinks=False)
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o022:
            return False
    info = os.stat(expected, follow_symlinks=False)
    return info.st_gid == 0 and stat.S_IMODE(info.st_mode) == 0o550 and bool(os.statvfs(expected).f_flag & os.ST_RDONLY)


def credential_paths(directory, release=None):
    if not directory or not Path(directory).is_absolute():
        raise SigningError("CREDENTIALS_DIRECTORY must name the systemd runtime credential directory")
    paths = {field: str(Path(directory) / name) for field, name in CREDENTIALS.items()}
    identities = set()
    try:
        for path in paths.values():
            info = os.stat(path, follow_symlinks=False)
            private = not info.st_mode & 0o077 and info.st_uid in (0, os.geteuid())
            managed = info.st_uid == 0 and info.st_gid == 0 and stat.S_IMODE(info.st_mode) == 0o440
            if not stat.S_ISREG(info.st_mode) or not (private or managed and systemd_credentials(directory, release)):
                raise SigningError("runtime credentials must be private regular files or root-owned read-only systemd credentials")
            identities.add((info.st_dev, info.st_ino))
    except OSError:
        raise SigningError("runtime credential files are unavailable") from None
    if len(identities) != len(paths):
        raise SigningError("runtime credentials must be distinct files, including both password files")
    return paths


def collect_diagnostic(fd, target):
    with os.fdopen(fd, "rb") as stream:
        while chunk := stream.read(4096):
            target.extend(chunk[:max(0, 8192 - len(target))])


def signing_error_category(arguments, diagnostic):
    text = bytes(diagnostic).lower()
    paths = []
    for index, value in enumerate(arguments[:-1]):
        if value in ("--ks", "--ks-pass", "--key-pass"):
            paths.append(os.fsencode(arguments[index + 1].removeprefix("file:")).lower())
    if any(value in text for value in (b"permission denied", b"accessdeniedexception")) and any(path in text for path in paths):
        return "credential-access-refused"
    if any(value in text for value in (b"keystore password was incorrect", b"keystore was tampered with", b"password verification failed",
                                       b"failed to decrypt safe contents", b"invalid keystore format", b"unrecognized keystore format")):
        return "keystore-format-or-password"
    if any(value in text for value in (b"does not contain a key with alias", b"does not contain key with alias", b"no key found with alias", b"not a key entry")):
        return "alias-or-key-rejected"
    if any(value in text for value in (b"could not create the java virtual machine", b"could not reserve enough space", b"error while loading shared libraries")):
        return "runtime-initialization-failed"
    return "unknown-tool-error"


def run_tool(arguments, work, operation):
    environment = {"LANG": "C.UTF-8", "LC_ALL": "C.UTF-8", "HOME": str(work), "TMPDIR": str(work), "PATH": "/nonexistent"}
    read_fd, write_fd = os.pipe()
    diagnostic = bytearray()
    reader = threading.Thread(target=collect_diagnostic, args=(read_fd, diagnostic), daemon=True)
    reader.start()
    failure = None
    try:
        result = subprocess.run(arguments, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=write_fd,
                                env=environment, timeout=120, check=True, text=True)
    except (OSError, subprocess.SubprocessError) as error:
        failure = error
    finally:
        os.close(write_fd)
        reader.join()
    if isinstance(failure, OSError):
        raise SigningError(operation + " failed (launch-error, errno=" + str(failure.errno or 0) + ")") from None
    if isinstance(failure, subprocess.TimeoutExpired):
        raise SigningError(operation + " failed (timeout)") from None
    if isinstance(failure, subprocess.CalledProcessError):
        if failure.returncode < 0:
            category, code = "signal", -failure.returncode
        else:
            category = signing_error_category(arguments, diagnostic) if operation == "APK signing" else "tool-exit"
            code = failure.returncode
        raise SigningError(operation + " failed (" + category + ", code=" + str(code) + ")") from None
    if failure is not None:
        raise SigningError(operation + " failed (unknown-launch-error)") from None
    return result.stdout


def apk_metadata(path, work):
    output = run_tool([AAPT2, "dump", "badging", str(path)], work, "APK package inspection")
    match = re.match(r"package: name='([^']*)' versionCode='([0-9]+)' versionName='([^']*)'", output)
    if not match or match[1] != PACKAGE or int(match[2]) < 1:
        raise SigningError("artifact must be the tv.aguatv.player APK with a positive versionCode")
    return {"package": PACKAGE, "versionCode": int(match[2]), "versionName": match[3]}


def checksum(path):
    with open(path, "rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def copy_unsigned(fd, target):
    with os.fdopen(fd, "rb") as source, open(target, "xb") as destination:
        before = os.fstat(source.fileno())
        if before.st_size < 1 or before.st_size > MAX_APK:
            raise SigningError("unsigned APK size is outside the accepted range")
        total = 0
        while chunk := source.read(1024 * 1024):
            total += len(chunk)
            if total > MAX_APK:
                raise SigningError("unsigned APK exceeds the size limit")
            destination.write(chunk)
        after = os.fstat(source.fileno())
        fields = ("st_dev", "st_ino", "st_size", "st_mtime_ns", "st_ctime_ns")
        if total != before.st_size or any(getattr(before, field) != getattr(after, field) for field in fields):
            raise SigningError("unsigned APK changed while being copied")


def publish(root_fd, temporary, release):
    libc = ctypes.CDLL(None, use_errno=True)
    rename = libc.renameat2
    rename.argtypes = [ctypes.c_int, ctypes.c_char_p, ctypes.c_int, ctypes.c_char_p, ctypes.c_uint]
    rename.restype = ctypes.c_int
    if rename(root_fd, os.fsencode(temporary), root_fd, os.fsencode(release), 1) != 0:
        raise SigningError("cannot publish release; existing releases are never replaced")
    os.fsync(root_fd)


def sign_locked(config, release, credentials, root, root_fd):
    try:
        os.stat(release, dir_fd=root_fd, follow_symlinks=False)
    except FileNotFoundError:
        pass
    else:
        raise SigningError("release already exists; refusing to replace it")
    inbox = Path(config["inboxDirectory"]) / release
    receipt = read_json(open_approved(inbox / "receipt.json"))
    fields = {"schemaVersion", "releaseId", "sourceRevision", "unsignedSha256"}
    if (set(receipt) != fields or type(receipt["schemaVersion"]) is not int or receipt["schemaVersion"] != 1
            or receipt["releaseId"] != release or not isinstance(receipt["sourceRevision"], str)
            or not REVISION.fullmatch(receipt["sourceRevision"]) or not isinstance(receipt["unsignedSha256"], str)
            or not HEX256.fullmatch(receipt["unsignedSha256"])):
        raise SigningError("release receipt is invalid")
    temporary = ".tmp-" + release
    os.mkdir(temporary, 0o700, dir_fd=root_fd)
    work = root / temporary
    try:
        unsigned = work / "unsigned.apk"
        copy_unsigned(open_approved(inbox / "unsigned.apk"), unsigned)
        if checksum(unsigned) != receipt["unsignedSha256"]:
            raise SigningError("unsigned APK checksum does not match the approved receipt")
        metadata = apk_metadata(unsigned, work)
        signed = work / "aguatv-player.apk"
        run_tool([APKSIGNER, "sign", "--ks", credentials["runtimeKeystoreFile"], "--ks-key-alias", "home",
                  "--ks-pass", "file:" + credentials["storePasswordFile"],
                  "--key-pass", "file:" + credentials["keyPasswordFile"], "--v4-signing-enabled", "false",
                  "--out", str(signed), str(unsigned)], work, "APK signing")
        verification = run_tool([APKSIGNER, "verify", "--verbose", "--print-certs", str(signed)], work, "APK signature verification")
        certificates = re.findall(r"^Signer #[0-9]+ certificate SHA-256 digest: ([0-9a-fA-F]{64})$", verification, re.MULTILINE)
        if len(certificates) != 1 or certificates[0].lower() != config["expectedCertificateSha256"]:
            raise SigningError("signed APK certificate does not match the pinned certificate")
        if apk_metadata(signed, work) != metadata:
            raise SigningError("signed APK metadata changed")
        manifest = {"schemaVersion": 1, "releaseId": release, "sourceRevision": receipt["sourceRevision"],
                    "unsignedSha256": receipt["unsignedSha256"], "signedSha256": checksum(signed),
                    "certSha256": certificates[0].lower(), **metadata,
                    "signedAt": datetime.datetime.now(datetime.timezone.utc).isoformat().replace("+00:00", "Z")}
        unsigned.unlink()
        with open(work / "manifest.json", "x", encoding="utf-8") as stream:
            json.dump(manifest, stream, sort_keys=True, separators=(",", ":"))
            stream.write("\n")
            stream.flush()
            os.fsync(stream.fileno())
        with open(signed, "rb") as stream:
            os.fsync(stream.fileno())
        if {entry.name for entry in work.iterdir()} != {"aguatv-player.apk", "manifest.json"}:
            raise SigningError("unexpected signing sidecar; refusing to publish")
        work_fd = os.open(work, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        try:
            os.fsync(work_fd)
        finally:
            os.close(work_fd)
        publish(root_fd, temporary, release)
        return manifest
    finally:
        if work.exists():
            shutil.rmtree(work)


def sign_release(config, release, directory):
    release_id(release)
    credentials = credential_paths(directory, release)
    root = Path(config["outputDirectory"]).resolve(strict=True)
    root_fd = os.open(root, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        info = os.fstat(root_fd)
        if info.st_uid != os.geteuid() or info.st_mode & 0o077:
            raise SigningError("output directory must be private and owned by the signer")
        lock_fd = os.open(".signer.lock", os.O_RDWR | os.O_CREAT | os.O_NOFOLLOW, 0o600, dir_fd=root_fd)
        try:
            lock = os.fstat(lock_fd)
            if not stat.S_ISREG(lock.st_mode) or lock.st_uid != os.geteuid() or lock.st_mode & 0o077 or lock.st_nlink != 1:
                raise SigningError("signer lock must be a private regular file")
            fcntl.flock(lock_fd, fcntl.LOCK_EX)
            return sign_locked(config, release, credentials, root, root_fd)
        finally:
            os.close(lock_fd)
    finally:
        os.close(root_fd)


def main():
    parser = argparse.ArgumentParser(prog="aguatv-sign-player", description="Sign one root-approved tv.aguatv.player release; never install it.")
    parser.add_argument("--version", action="version", version="aguatv-sign-player 1")
    parser.add_argument("release_id", type=release_id)
    arguments = parser.parse_args()
    os.umask(0o077)
    try:
        config = load_config(os.environ.get("AGUATV_SIGNER_CONFIG", CONFIG))
        manifest = sign_release(config, arguments.release_id, os.environ.get("CREDENTIALS_DIRECTORY"))
    except (SigningError, OSError, ValueError) as error:
        message = str(error) if isinstance(error, SigningError) else "filesystem or configuration operation failed"
        print("aguatv-sign-player: " + message, file=sys.stderr)
        return 1
    print(json.dumps(manifest, sort_keys=True, separators=(",", ":")))
    return 0


if __name__ == "__main__":
    sys.exit(main())
