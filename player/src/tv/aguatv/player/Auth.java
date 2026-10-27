package tv.aguatv.player;

import org.json.JSONException;
import org.json.JSONObject;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;

final class Auth {
    static final String CLIENT = "aguatv-tv";
    static final String SCOPE = "openid profile groups offline_access";
    private static final int TIMEOUT_MS = 15000;
    private static final int MAX_BODY = 256 * 1024;

    static final class Device {
        final String deviceCode, userCode, uri, uriComplete;
        final long expiresAt;
        int interval;

        Device(String deviceCode, String userCode, String uri, String uriComplete, long expiresAt, int interval) {
            this.deviceCode = deviceCode;
            this.userCode = userCode;
            this.uri = uri;
            this.uriComplete = uriComplete;
            this.expiresAt = expiresAt;
            this.interval = interval;
        }
    }

    static final class Tokens {
        final String access, refresh;
        final long expiresAt;

        Tokens(String access, String refresh, long expiresAt) {
            this.access = access;
            this.refresh = refresh;
            this.expiresAt = expiresAt;
        }
    }

    static final class Refused extends IOException {
        private static final long serialVersionUID = 1L;
        final String error;

        Refused(String error) {
            super(error);
            this.error = error;
        }
    }

    private final Fence fence;
    private String deviceEndpoint, tokenEndpoint, revokeEndpoint;

    Auth(Fence fence) {
        this.fence = fence;
    }

    private synchronized void discover() throws IOException {
        if (tokenEndpoint != null) return;
        JSONObject d = call("GET", fence.authOrigin + "/.well-known/openid-configuration", null);
        String device = d.optString("device_authorization_endpoint", null);
        String token = d.optString("token_endpoint", null);
        String revoke = d.optString("revocation_endpoint", null);
        if (device == null || token == null || !fence.authority(device) || !fence.authority(token)) {
            throw new IOException("the portal's discovery names endpoints elsewhere");
        }
        deviceEndpoint = device;
        tokenEndpoint = token;
        revokeEndpoint = revoke != null && fence.authority(revoke) ? revoke : null;
    }

    Device start() throws IOException {
        discover();
        JSONObject r = call("POST", deviceEndpoint, form("client_id", CLIENT, "scope", SCOPE));
        try {
            String uri = r.getString("verification_uri");
            String complete = r.optString("verification_uri_complete", uri);
            if (!fence.authority(uri) || !fence.authority(complete)) throw new IOException("verification is not on the portal");
            return new Device(r.getString("device_code"), r.getString("user_code"), uri, complete,
                System.currentTimeMillis() + r.optLong("expires_in", 600) * 1000, Math.max(5, r.optInt("interval", 5)));
        } catch (JSONException e) {
            throw new IOException("device authorization: " + e.getMessage());
        }
    }

    Tokens poll(Device d) throws IOException {
        discover();
        return tokens(call("POST", tokenEndpoint,
            form("grant_type", "urn:ietf:params:oauth:grant-type:device_code", "device_code", d.deviceCode, "client_id", CLIENT)), null);
    }

    Tokens refresh(String refresh) throws IOException {
        discover();
        return tokens(call("POST", tokenEndpoint, form("grant_type", "refresh_token", "refresh_token", refresh, "client_id", CLIENT)), refresh);
    }

    void revoke(String refresh) {
        try {
            discover();
            if (revokeEndpoint != null) call("POST", revokeEndpoint, form("token", refresh, "token_type_hint", "refresh_token", "client_id", CLIENT));
        } catch (IOException e) {
            return;
        }
    }

    private static Tokens tokens(JSONObject r, String previous) throws IOException {
        String access = r.optString("access_token", null);
        if (access == null) throw new IOException("no access token");
        String refresh = r.optString("refresh_token", previous);
        if (refresh == null) throw new IOException("no refresh token: the client needs offline_access");
        return new Tokens(access, refresh, System.currentTimeMillis() + r.optLong("expires_in", 300) * 1000);
    }

    private static String form(String... kv) {
        StringBuilder b = new StringBuilder();
        try {
            for (int i = 0; i < kv.length; i += 2) {
                if (b.length() > 0) b.append('&');
                b.append(URLEncoder.encode(kv[i], "UTF-8")).append('=').append(URLEncoder.encode(kv[i + 1], "UTF-8"));
            }
        } catch (IOException e) {
            throw new IllegalStateException(e);
        }
        return b.toString();
    }

    private JSONObject call(String method, String url, String body) throws IOException {
        if (!fence.authority(url)) throw new IOException("refusing to talk to " + url);
        HttpURLConnection c = (HttpURLConnection) new URL(url).openConnection();
        try {
            c.setInstanceFollowRedirects(false);
            c.setUseCaches(false);
            c.setConnectTimeout(TIMEOUT_MS);
            c.setReadTimeout(TIMEOUT_MS);
            c.setRequestMethod(method);
            c.setRequestProperty("Accept", "application/json");
            if (body != null) {
                c.setDoOutput(true);
                c.setRequestProperty("Content-Type", "application/x-www-form-urlencoded");
                try (OutputStream o = c.getOutputStream()) {
                    o.write(body.getBytes(StandardCharsets.UTF_8));
                }
            }
            int code = c.getResponseCode();
            String text = read(code >= 400 ? c.getErrorStream() : c.getInputStream());
            JSONObject j;
            try {
                j = text.isEmpty() ? new JSONObject() : new JSONObject(text);
            } catch (JSONException e) {
                throw new IOException("the portal answered " + code + " without JSON");
            }
            if (code >= 400) throw new Refused(j.optString("error", "http_" + code));
            if (code != 200) throw new IOException("the portal answered " + code);
            return j;
        } finally {
            c.disconnect();
        }
    }

    private static String read(InputStream in) throws IOException {
        if (in == null) return "";
        try (InputStream s = in) {
            ByteArrayOutputStream out = new ByteArrayOutputStream();
            byte[] buf = new byte[8192];
            for (int n; (n = s.read(buf)) > 0; ) {
                out.write(buf, 0, n);
                if (out.size() > MAX_BODY) throw new IOException("answer too large");
            }
            return new String(out.toByteArray(), StandardCharsets.UTF_8);
        }
    }
}
