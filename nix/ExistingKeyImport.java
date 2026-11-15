import java.io.Console;
import java.io.IOException;
import java.nio.ByteBuffer;
import java.nio.CharBuffer;
import java.nio.channels.SeekableByteChannel;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.LinkOption;
import java.nio.file.Path;
import java.nio.file.StandardOpenOption;
import java.nio.file.attribute.PosixFilePermissions;
import java.security.Key;
import java.security.KeyStore;
import java.security.MessageDigest;
import java.security.PrivateKey;
import java.security.cert.Certificate;
import java.util.Arrays;
import java.util.HexFormat;
import java.util.List;
import java.util.Set;
import java.util.concurrent.TimeUnit;

public final class ExistingKeyImport {
    static final String PIN = "642be2dcf5893a7e7024d32731f90aebf5f329c938bd48fdbf79ce1c52d20d44";
    static final String[] NAMES = {"keystore", "store-password", "key-password"};
    static final int MAX_KEYSTORE = 16 * 1024 * 1024;

    static final class Refusal extends Exception {
        final String status;
        Refusal(String status) { this.status = status; }
    }

    static byte[] snapshot(Path source) throws Refusal {
        try {
            int uid = ((Number) Files.getAttribute(Path.of("/proc/self"), "unix:uid")).intValue();
            int owner = ((Number) Files.getAttribute(source, "unix:uid", LinkOption.NOFOLLOW_LINKS)).intValue();
            int mode = ((Number) Files.getAttribute(source, "unix:mode", LinkOption.NOFOLLOW_LINKS)).intValue();
            if (uid == 0 || owner != uid || (mode & 077) != 0 || !Files.isRegularFile(source, LinkOption.NOFOLLOW_LINKS)) {
                throw new Refusal("PRIVATE_OPERATOR_KEYSTORE_REQUIRED");
            }
            try (SeekableByteChannel channel = Files.newByteChannel(source, Set.of(StandardOpenOption.READ, LinkOption.NOFOLLOW_LINKS))) {
                if (channel.size() < 1 || channel.size() > MAX_KEYSTORE) throw new Refusal("KEYSTORE_SIZE_REFUSED");
                ByteBuffer buffer = ByteBuffer.allocate((int) channel.size());
                try {
                    while (buffer.hasRemaining() && channel.read(buffer) != -1) { }
                    if (buffer.hasRemaining() || channel.read(ByteBuffer.allocate(1)) != -1) throw new Refusal("KEYSTORE_CHANGED");
                    return buffer.array().clone();
                } finally {
                    Arrays.fill(buffer.array(), (byte) 0);
                }
            }
        } catch (IOException | RuntimeException error) {
            throw new Refusal("KEYSTORE_UNAVAILABLE");
        }
    }

    static void validate(byte[] original, char[] storePassword, char[] keyPassword, String expectedPin) throws Refusal {
        if (storePassword == null || keyPassword == null || storePassword.length == 0 || keyPassword.length == 0
                || storePassword.length > 1024 || keyPassword.length > 1024) throw new Refusal("PASSWORD_INPUT_REFUSED");
        KeyStore loaded = null;
        for (String type : List.of("PKCS12", "JKS")) {
            try {
                KeyStore candidate = KeyStore.getInstance(type);
                candidate.load(new java.io.ByteArrayInputStream(original), storePassword);
                loaded = candidate;
                break;
            } catch (java.security.GeneralSecurityException | IOException ignored) { }
        }
        if (loaded == null) throw new Refusal("STORE_FORMAT_OR_PASSWORD_REJECTED");
        try {
            Key key = loaded.getKey("home", keyPassword);
            if (!(key instanceof PrivateKey)) throw new Refusal("PRIVATE_KEY_ALIAS_REQUIRED");
        } catch (java.security.GeneralSecurityException error) {
            throw new Refusal("KEY_UNLOCK_FAILED");
        }
        try {
            Certificate certificate = loaded.getCertificate("home");
            if (certificate == null) throw new Refusal("PINNED_CERTIFICATE_REQUIRED");
            String actual = HexFormat.of().formatHex(MessageDigest.getInstance("SHA-256").digest(certificate.getEncoded()));
            if (!actual.equals(expectedPin)) throw new Refusal("CERTIFICATE_PIN_MISMATCH");
        } catch (java.security.GeneralSecurityException error) {
            throw new Refusal("CERTIFICATE_CHECK_FAILED");
        }
    }

    static byte[] passwordBytes(char[] password) throws Refusal {
        ByteBuffer encoded;
        try {
            encoded = StandardCharsets.UTF_8.newEncoder().encode(CharBuffer.wrap(password));
        } catch (java.nio.charset.CharacterCodingException error) {
            throw new Refusal("PASSWORD_ENCODING_REFUSED");
        }
        try {
            byte[] result = new byte[encoded.remaining()];
            encoded.get(result);
            return result;
        } finally {
            if (encoded.hasArray()) Arrays.fill(encoded.array(), (byte) 0);
        }
    }

    static void encrypt(byte[] data, String name, Path stage, List<String> encryptor, Path config) throws Refusal {
        Path output = stage.resolve("aguatv-release-" + name + ".enc.json");
        try {
            Files.createFile(output, PosixFilePermissions.asFileAttribute(PosixFilePermissions.fromString("rw-------")));
            java.util.ArrayList<String> arguments = new java.util.ArrayList<>(encryptor);
            arguments.addAll(List.of("--config", config.toString(), "encrypt", "--input-type", "binary", "--output-type", "json",
                    "--filename-override", "secrets/aguatv-release-" + name + ".enc.json"));
            ProcessBuilder builder = new ProcessBuilder(arguments);
            builder.directory(stage.toFile());
            builder.environment().clear();
            builder.environment().put("HOME", stage.toString());
            builder.environment().put("PATH", "/nonexistent");
            builder.environment().put("LANG", "C.UTF-8");
            builder.redirectError(ProcessBuilder.Redirect.DISCARD);
            builder.redirectOutput(output.toFile());
            Process child = builder.start();
            java.util.concurrent.atomic.AtomicBoolean inputFailed = new java.util.concurrent.atomic.AtomicBoolean();
            Thread writer = new Thread(() -> {
                try (java.io.OutputStream input = child.getOutputStream()) { input.write(data); }
                catch (IOException error) { inputFailed.set(true); }
            });
            writer.setDaemon(true);
            writer.start();
            try {
                if (!child.waitFor(60, TimeUnit.SECONDS) || child.exitValue() != 0) throw new Refusal("ENCRYPTION_FAILED");
                writer.join(1000);
                if (writer.isAlive() || inputFailed.get() || Files.size(output) == 0) throw new Refusal("ENCRYPTION_FAILED");
            } finally {
                if (child.isAlive()) {
                    child.destroyForcibly();
                    child.waitFor(5, TimeUnit.SECONDS);
                }
                writer.join(1000);
            }
        } catch (IOException | RuntimeException error) {
            throw new Refusal("ENCRYPTION_FAILED");
        } catch (InterruptedException error) {
            Thread.currentThread().interrupt();
            throw new Refusal("ENCRYPTION_FAILED");
        }
    }

    static void importValidated(byte[] original, char[] storePassword, char[] keyPassword, String expectedPin,
                                Path stage, List<String> encryptor, Path config) throws Refusal {
        validate(original, storePassword, keyPassword, expectedPin);
        boolean created = false;
        byte[] storeBytes = null;
        byte[] keyBytes = null;
        boolean complete = false;
        try {
            Files.createDirectory(stage, PosixFilePermissions.asFileAttribute(PosixFilePermissions.fromString("rwx------")));
            created = true;
            storeBytes = passwordBytes(storePassword);
            keyBytes = passwordBytes(keyPassword);
            encrypt(original, "keystore", stage, encryptor, config);
            encrypt(storeBytes, "store-password", stage, encryptor, config);
            encrypt(keyBytes, "key-password", stage, encryptor, config);
            complete = true;
        } catch (IOException | RuntimeException error) {
            throw new Refusal("FRESH_PRIVATE_STAGING_REQUIRED");
        } finally {
            if (storeBytes != null) Arrays.fill(storeBytes, (byte) 0);
            if (keyBytes != null) Arrays.fill(keyBytes, (byte) 0);
            if (created && !complete) {
                try {
                    for (String name : NAMES) Files.deleteIfExists(stage.resolve("aguatv-release-" + name + ".enc.json"));
                    Files.delete(stage);
                } catch (IOException error) {
                    throw new Refusal("ENCRYPTED_STAGING_CLEANUP_REQUIRED");
                }
            }
        }
    }

    static Path staging() throws Refusal {
        Path base = Path.of(System.getProperty("user.home"), ".local", "state", "aguatv-key-import");
        try {
            Files.createDirectories(base, PosixFilePermissions.asFileAttribute(PosixFilePermissions.fromString("rwx------")));
            int uid = ((Number) Files.getAttribute(Path.of("/proc/self"), "unix:uid")).intValue();
            int owner = ((Number) Files.getAttribute(base, "unix:uid", LinkOption.NOFOLLOW_LINKS)).intValue();
            int mode = ((Number) Files.getAttribute(base, "unix:mode", LinkOption.NOFOLLOW_LINKS)).intValue();
            if (uid == 0 || owner != uid || (mode & 0777) != 0700 || !Files.isDirectory(base, LinkOption.NOFOLLOW_LINKS)) {
                throw new Refusal("PRIVATE_OPERATOR_STAGING_REQUIRED");
            }
            return base.resolve("validated");
        } catch (IOException | RuntimeException error) {
            throw new Refusal("PRIVATE_OPERATOR_STAGING_REQUIRED");
        }
    }

    public static void main(String[] arguments) {
        if (arguments.length != 0) {
            System.err.println("NO_ARGUMENTS_ACCEPTED");
            System.exit(2);
        }
        Console console = System.console();
        if (console == null) {
            System.err.println("INTERACTIVE_CONSOLE_REQUIRED");
            System.exit(2);
        }
        char[] storePassword = null;
        char[] keyPassword = null;
        byte[] original = null;
        int exit = 0;
        try {
            if (((Number) Files.getAttribute(Path.of("/proc/self"), "unix:uid")).intValue() == 0) throw new Refusal("RUN_AS_OPERATOR_NOT_ROOT");
            storePassword = console.readPassword("Existing keystore STORE password: ");
            keyPassword = console.readPassword("Existing private-key password (alias home): ");
            original = snapshot(Path.of(System.getProperty("user.home"), ".local", "share", "aguatv", "release.keystore"));
            importValidated(original, storePassword, keyPassword, PIN, staging(), List.of("@sops@"), Path.of("@config@"));
            System.out.println("VALIDATED_AND_ENCRYPTED");
            System.out.println("CERTIFICATE_SHA256=" + PIN);
        } catch (Refusal error) {
            System.err.println(error.status);
            exit = 1;
        } catch (Throwable error) {
            System.err.println("IMPORT_REFUSED");
            exit = 1;
        } finally {
            if (storePassword != null) Arrays.fill(storePassword, '\0');
            if (keyPassword != null) Arrays.fill(keyPassword, '\0');
            if (original != null) Arrays.fill(original, (byte) 0);
        }
        System.exit(exit);
    }
}
