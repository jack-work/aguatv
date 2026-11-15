import java.nio.file.Files;
import java.nio.file.Path;
import java.security.KeyStore;
import java.security.MessageDigest;
import java.util.Arrays;
import java.util.HexFormat;
import java.util.List;

public final class ExistingKeyImportTest {
    static final char[] STORE = " fixture store ".toCharArray();
    static final char[] KEY = " fixture key ".toCharArray();

    static void require(boolean condition, String label) {
        if (!condition) throw new AssertionError(label);
    }

    static void refused(byte[] original, char[] store, char[] key, String pin, Path stage,
                        List<String> encryptor, Path config, String status) throws Exception {
        try {
            ExistingKeyImport.importValidated(original, store, key, pin, stage, encryptor, config);
            throw new AssertionError("expected refusal");
        } catch (ExistingKeyImport.Refusal error) {
            require(error.status.equals(status), "fixed status differs");
        }
        require(!Files.exists(stage), "refusal left staged outputs");
    }

    public static void main(String[] arguments) throws Exception {
        Path source = Path.of(arguments[0]);
        Path root = Path.of(arguments[1]);
        Path python = Path.of(arguments[2]);
        Path stub = Path.of(arguments[3]);
        Path fail = Path.of(arguments[4]);
        Path config = Path.of(arguments[5]);
        Path sops = Path.of(arguments[6]);
        byte[] original = Files.readAllBytes(source);
        byte[] before = original.clone();
        KeyStore keystore = KeyStore.getInstance("JKS");
        keystore.load(new java.io.ByteArrayInputStream(original), STORE);
        String pin = HexFormat.of().formatHex(MessageDigest.getInstance("SHA-256").digest(keystore.getCertificate("home").getEncoded()));
        List<String> fake = List.of(python.toString(), stub.toString());
        refused(original, "incorrect-store".toCharArray(), KEY, pin, root.resolve("bad-store"), fake, config, "STORE_FORMAT_OR_PASSWORD_REJECTED");
        refused(original, STORE, "incorrect-key".toCharArray(), pin, root.resolve("bad-key"), fake, config, "KEY_UNLOCK_FAILED");
        refused(original, STORE, KEY, "0".repeat(64), root.resolve("bad-pin"), fake, config, "CERTIFICATE_PIN_MISMATCH");
        refused(original, STORE, KEY, pin, root.resolve("sops-failed"), List.of(python.toString(), fail.toString()), config, "ENCRYPTION_FAILED");
        Path success = root.resolve("mock-success");
        ExistingKeyImport.importValidated(original, STORE, KEY, pin, success, fake, config);
        try (var files = Files.list(success)) { require(files.count() == 3, "not exactly three ciphertext outputs"); }
        require(Files.getPosixFilePermissions(success).equals(java.nio.file.attribute.PosixFilePermissions.fromString("rwx------")), "staging is not private");
        Path sentinel = success.resolve("aguatv-release-keystore.enc.json");
        byte[] sentinelBefore = Files.readAllBytes(sentinel);
        try {
            ExistingKeyImport.importValidated(original, STORE, KEY, pin, success, fake, config);
            throw new AssertionError("existing staging was overwritten");
        } catch (ExistingKeyImport.Refusal error) {
            require(error.status.equals("FRESH_PRIVATE_STAGING_REQUIRED"), "wrong existing-stage refusal");
        }
        require(Arrays.equals(sentinelBefore, Files.readAllBytes(sentinel)), "existing ciphertext changed");
        ExistingKeyImport.importValidated(original, STORE, KEY, pin, root.resolve("real-sops"), List.of(sops.toString()), config);
        require(Arrays.equals(before, original) && Arrays.equals(before, Files.readAllBytes(source)), "original keystore changed");
        require(Arrays.equals(ExistingKeyImport.passwordBytes(STORE), " fixture store ".getBytes(java.nio.charset.StandardCharsets.UTF_8)), "password bytes changed");
        byte[] pkcs12 = Files.readAllBytes(root.resolve("fixture.p12"));
        KeyStore p12 = KeyStore.getInstance("PKCS12");
        char[] password = "fixture-p12".toCharArray();
        p12.load(new java.io.ByteArrayInputStream(pkcs12), password);
        String p12Pin = HexFormat.of().formatHex(MessageDigest.getInstance("SHA-256").digest(p12.getCertificate("home").getEncoded()));
        ExistingKeyImport.validate(pkcs12, password, password, p12Pin);
        System.out.println("PASS store/key rejection, pin mismatch, SOPS failure cleanup, same bytes, no overwrite, real encryption and JKS/PKCS12 validation");
    }
}
