package tv.aguatv.player;

import android.content.Context;
import android.content.SharedPreferences;
import android.security.keystore.KeyGenParameterSpec;
import android.security.keystore.KeyProperties;
import android.util.Base64;

import java.nio.charset.StandardCharsets;
import java.security.KeyStore;
import java.util.Map;

import javax.crypto.Cipher;
import javax.crypto.KeyGenerator;
import javax.crypto.SecretKey;
import javax.crypto.spec.GCMParameterSpec;

final class Vault {
    private static final String ALIAS = "aguatv-receiver";
    private static final String BLOB = "sealed";

    static final class Secrets {
        final String refresh;
        final String screen;

        Secrets(String refresh, String screen) {
            this.refresh = refresh;
            this.screen = screen;
        }
    }

    private final SharedPreferences prefs;

    Vault(Context context) {
        prefs = context.getSharedPreferences("vault", Context.MODE_PRIVATE);
    }

    synchronized Secrets get() {
        String sealed = prefs.getString(BLOB, null);
        if (sealed == null) return null;
        try {
            int colon = sealed.indexOf(':');
            byte[] iv = Base64.decode(sealed.substring(0, colon), Base64.NO_WRAP);
            byte[] ct = Base64.decode(sealed.substring(colon + 1), Base64.NO_WRAP);
            Cipher c = Cipher.getInstance("AES/GCM/NoPadding");
            c.init(Cipher.DECRYPT_MODE, key(), new GCMParameterSpec(128, iv));
            Object parsed = Json.parse(new String(c.doFinal(ct), StandardCharsets.UTF_8));
            if (!(parsed instanceof Map)) return null;
            Map<?, ?> m = (Map<?, ?>) parsed;
            Object r = m.get("r"), s = m.get("s");
            if (!(r instanceof String)) return null;
            return new Secrets((String) r, s instanceof String ? (String) s : null);
        } catch (Exception e) {
            return null;
        }
    }

    synchronized boolean put(Secrets s) {
        try {
            String plain = "{\"r\":" + Json.quote(s.refresh) + (s.screen == null ? "" : ",\"s\":" + Json.quote(s.screen)) + "}";
            Cipher c = Cipher.getInstance("AES/GCM/NoPadding");
            c.init(Cipher.ENCRYPT_MODE, key());
            byte[] ct = c.doFinal(plain.getBytes(StandardCharsets.UTF_8));
            String sealed = Base64.encodeToString(c.getIV(), Base64.NO_WRAP) + ":" + Base64.encodeToString(ct, Base64.NO_WRAP);
            return prefs.edit().putString(BLOB, sealed).commit();
        } catch (Exception e) {
            return false;
        }
    }

    synchronized void clear() {
        prefs.edit().remove(BLOB).commit();
    }

    private static SecretKey key() throws Exception {
        KeyStore ks = KeyStore.getInstance("AndroidKeyStore");
        ks.load(null);
        if (ks.containsAlias(ALIAS)) return ((KeyStore.SecretKeyEntry) ks.getEntry(ALIAS, null)).getSecretKey();
        KeyGenerator g = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore");
        g.init(new KeyGenParameterSpec.Builder(ALIAS, KeyProperties.PURPOSE_ENCRYPT | KeyProperties.PURPOSE_DECRYPT)
            .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
            .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
            .setKeySize(256)
            .build());
        return g.generateKey();
    }
}
