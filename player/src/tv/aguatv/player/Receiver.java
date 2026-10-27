package tv.aguatv.player;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.Intent;
import android.content.SharedPreferences;
import android.graphics.Bitmap;
import android.graphics.Color;
import android.graphics.Typeface;
import android.net.Uri;
import android.net.http.SslError;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.KeyEvent;
import android.view.WindowManager;
import android.webkit.PermissionRequest;
import android.webkit.SslErrorHandler;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebMessage;
import android.webkit.WebMessagePort;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.FrameLayout;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.TextView;

import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URI;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.HashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public final class Receiver extends Activity {
    static final String EXTRA_APP = "app";
    static final String EXTRA_AUTH = "auth";
    static final String EXTRA_ALSO = "also";
    private static final String PREFS = "receiver";
    private static final String SCREEN_COOKIE = "aguatv_screen";
    private static final int STAGE = 0xFF1A0D0F, INK = 0xFFF0DFC8, MUTED = 0xFFA5887B, GOLD = 0xFFD9A94E;
    private static final int TIMEOUT_MS = 30000;
    private static final int MAX_REPLY = 64 * 1024;

    private final Handler ui = new Handler(Looper.getMainLooper());
    private final ExecutorService writes = Executors.newSingleThreadExecutor();
    private final Ops ops = new Ops(System::currentTimeMillis);
    private final TvInputs inputs = new TvInputs(this);
    private FrameLayout root;
    private WebView web;
    private Fence fence;
    private Vault vault;
    private Auth auth;
    private volatile Thread signIn;
    private volatile boolean menuHeld;

    private final Object tokenLock = new Object();
    private String access;
    private long accessExpires;
    private WebMessagePort[] ports;

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        WebView.setWebContentsDebuggingEnabled(false);
        SharedPreferences prefs = getSharedPreferences(PREFS, MODE_PRIVATE);
        String[] stored = { prefs.getString(EXTRA_APP, null), prefs.getString(EXTRA_AUTH, null), prefs.getString(EXTRA_ALSO, null) };
        Intent intent = getIntent();
        String[] given = { intent.getStringExtra(EXTRA_APP), intent.getStringExtra(EXTRA_AUTH), intent.getStringExtra(EXTRA_ALSO) };
        String[] chosen = Fence.firstUse(stored, given);
        if (chosen != null && chosen != stored) {
            prefs.edit().putString(EXTRA_APP, chosen[0]).putString(EXTRA_AUTH, chosen[1]).putString(EXTRA_ALSO, chosen[2]).apply();
        }
        fence = chosen == null ? null : Fence.of(chosen[0], chosen[1], chosen[2]);

        root = new FrameLayout(this);
        root.setBackgroundColor(STAGE);
        setContentView(root);
        if (fence == null) {
            root.addView(text("No app is set. From a computer: aguatv receiver <app URL> <sign-in URL>", 22, INK),
                new FrameLayout.LayoutParams(-1, -1));
            return;
        }
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        vault = new Vault(this);
        auth = new Auth(fence);
        if (vault.get() == null) showSignIn();
        else showApp();
    }

    private TextView text(String s, int sp, int color) {
        TextView t = new TextView(this);
        t.setText(s);
        t.setTextColor(color);
        t.setTextSize(TypedValue.COMPLEX_UNIT_SP, sp);
        t.setGravity(Gravity.CENTER);
        return t;
    }

    private void showSignIn() {
        ui.post(() -> {
            Thread running = signIn;
            if (running != null && running.isAlive()) return;
            if (web != null) {
                root.removeView(web);
                web.destroy();
                web = null;
            }
            closePorts();
            root.removeAllViews();
            LinearLayout row = new LinearLayout(this);
            row.setOrientation(LinearLayout.HORIZONTAL);
            row.setGravity(Gravity.CENTER);
            ImageView qr = new ImageView(this);
            qr.setBackgroundColor(Color.WHITE);
            int side = (int) TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_DIP, 360, getResources().getDisplayMetrics());
            row.addView(qr, new LinearLayout.LayoutParams(side, side));
            LinearLayout col = new LinearLayout(this);
            col.setOrientation(LinearLayout.VERTICAL);
            col.setPadding(side / 6, 0, 0, 0);
            TextView title = text("Sign in this TV", 30, INK);
            title.setGravity(Gravity.START);
            TextView code = text("…", 72, GOLD);
            code.setGravity(Gravity.START);
            code.setTypeface(Typeface.MONOSPACE, Typeface.BOLD);
            TextView how = text("Scan the code with your phone, or open the address below and enter the code.", 20, MUTED);
            how.setGravity(Gravity.START);
            TextView where = text("", 22, INK);
            where.setGravity(Gravity.START);
            TextView status = text("Asking the portal for a code…", 18, MUTED);
            status.setGravity(Gravity.START);
            col.addView(title);
            col.addView(code);
            col.addView(how);
            col.addView(where);
            col.addView(status);
            row.addView(col);
            root.addView(row, new FrameLayout.LayoutParams(-1, -1));
            Thread t = new Thread(() -> signInLoop(qr, code, where, status), "sign-in");
            signIn = t;
            t.start();
        });
    }

    private void signInLoop(ImageView qr, TextView code, TextView where, TextView status) {
        Thread me = Thread.currentThread();
        while (signIn == me) {
            try {
                Auth.Device d = auth.start();
                Bitmap bmp = render(QrCode.encode(d.uriComplete));
                ui.post(() -> {
                    qr.setImageBitmap(bmp);
                    code.setText(d.userCode);
                    where.setText(d.uri);
                    status.setText("Waiting for you to approve on your phone.");
                });
                while (signIn == me && System.currentTimeMillis() < d.expiresAt) {
                    Thread.sleep(d.interval * 1000L);
                    try {
                        Auth.Tokens t = auth.poll(d);
                        synchronized (tokenLock) {
                            if (!vault.put(new Vault.Secrets(t.refresh, null))) throw new IOException("could not store the sign-in");
                            access = t.access;
                            accessExpires = t.expiresAt;
                        }
                        signIn = null;
                        showApp();
                        return;
                    } catch (Auth.Refused r) {
                        if ("slow_down".equals(r.error)) d.interval += 5;
                        else if (!"authorization_pending".equals(r.error)) {
                            ui.post(() -> status.setText("The portal said " + r.error + ". Getting a new code."));
                            break;
                        }
                    }
                }
            } catch (InterruptedException e) {
                return;
            } catch (IOException | RuntimeException e) {
                String why = e.getMessage();
                ui.post(() -> status.setText("Cannot reach the portal (" + why + "). Trying again."));
                try {
                    Thread.sleep(10000);
                } catch (InterruptedException ie) {
                    return;
                }
            }
        }
    }

    private static Bitmap render(QrCode q) {
        int quiet = 4, scale = 8, side = (q.size + quiet * 2) * scale;
        int[] px = new int[side * side];
        for (int y = 0; y < side; y++) {
            for (int x = 0; x < side; x++) {
                px[y * side + x] = q.dark(x / scale - quiet, y / scale - quiet) ? Color.BLACK : Color.WHITE;
            }
        }
        return Bitmap.createBitmap(px, side, side, Bitmap.Config.RGB_565);
    }

    private String accessToken(boolean forceRefresh) {
        synchronized (tokenLock) {
            if (!forceRefresh && access != null && System.currentTimeMillis() < accessExpires - 30000) return access;
            Vault.Secrets s = vault.get();
            if (s == null) return null;
            try {
                Auth.Tokens t = auth.refresh(s.refresh);
                if (!vault.put(new Vault.Secrets(t.refresh, s.screen))) return null;
                access = t.access;
                accessExpires = t.expiresAt;
                return access;
            } catch (Auth.Refused r) {
                access = null;
                vault.clear();
                return null;
            } catch (IOException e) {
                return null;
            }
        }
    }

    private void signedOut() {
        synchronized (tokenLock) {
            access = null;
            vault.clear();
        }
        showSignIn();
    }

    private void showApp() {
        ui.post(() -> {
            root.removeAllViews();
            web = new WebView(this);
            web.setBackgroundColor(Color.BLACK);
            WebSettings s = web.getSettings();
            s.setJavaScriptEnabled(true);
            s.setDomStorageEnabled(true);
            s.setMediaPlaybackRequiresUserGesture(false);
            s.setAllowFileAccess(false);
            s.setAllowContentAccess(false);
            s.setAllowFileAccessFromFileURLs(false);
            s.setAllowUniversalAccessFromFileURLs(false);
            s.setGeolocationEnabled(false);
            s.setSupportMultipleWindows(false);
            s.setJavaScriptCanOpenWindowsAutomatically(false);
            s.setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);
            s.setSafeBrowsingEnabled(true);
            s.setCacheMode(WebSettings.LOAD_NO_CACHE);
            android.webkit.CookieManager.getInstance().setAcceptCookie(false);
            web.setDownloadListener((url, agent, disposition, mime, length) -> { });
            web.setWebChromeClient(new WebChromeClient() {
                @Override
                public void onPermissionRequest(PermissionRequest request) {
                    request.deny();
                }

                @Override
                public boolean onShowFileChooser(WebView view, ValueCallback<Uri[]> files, FileChooserParams params) {
                    files.onReceiveValue(null);
                    return true;
                }
            });
            web.setWebViewClient(new WebViewClient() {
                @Override
                public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest req) {
                    if (fence.opens(req.getUrl().toString())) return false;
                    view.loadUrl(fence.app);
                    return true;
                }

                @Override
                public void doUpdateVisitedHistory(WebView view, String url, boolean reload) {
                    if (!fence.opens(url)) view.loadUrl(fence.app);
                }

                @Override
                public void onPageStarted(WebView view, String url, Bitmap favicon) {
                    closePorts();
                }

                @Override
                public void onPageFinished(WebView view, String url) {
                    if (fence.opens(url) && fence.ownsApp(url)) openChannel(view);
                }

                @Override
                public WebResourceResponse shouldInterceptRequest(WebView view, WebResourceRequest req) {
                    String url = req.getUrl().toString();
                    if (!fence.ownsApp(url)) {
                        return req.isForMainFrame() && fence.opens(url) ? null : refused(403);
                    }
                    if (req.isForMainFrame() && !fence.opens(url)) return refused(403);
                    String method = req.getMethod().toUpperCase(Locale.ROOT);
                    if (!method.equals("GET") && !method.equals("HEAD")) return refused(403);
                    return fetch(url, method, req.getRequestHeaders(), req.isForMainFrame(), true);
                }

                @Override
                public void onReceivedSslError(WebView view, SslErrorHandler handler, SslError error) {
                    handler.cancel();
                }
            });
            root.addView(web, new FrameLayout.LayoutParams(-1, -1));
            web.loadUrl(fence.app);
            web.requestFocus();
        });
    }

    private void closePorts() {
        WebMessagePort[] old = ports;
        ports = null;
        if (old != null) for (WebMessagePort p : old) p.close();
    }

    private void openChannel(WebView view) {
        closePorts();
        WebMessagePort[] pair = view.createWebMessageChannel();
        ports = pair;
        pair[0].setWebMessageCallback(new WebMessagePort.WebMessageCallback() {
            @Override
            public void onMessage(WebMessagePort port, WebMessage message) {
                if (ports != pair) return;
                Ops.Request r = ops.accept(message.getData());
                if (r == null) return;
                writes.execute(() -> {
                    String reply = r.local() ? Ops.answer(r, inputs) : write(r);
                    ui.post(() -> {
                        if (ports == pair) port.postMessage(new WebMessage(reply));
                    });
                });
            }
        });
        view.postWebMessage(new WebMessage("aguatv-receiver", new WebMessagePort[] { pair[1] }), Uri.parse(fence.appOrigin));
    }

    private void backToPage() {
        WebMessagePort[] p = ports;
        if (p != null) p[0].postMessage(new WebMessage("{\"event\":\"back\"}"));
    }

    private String write(Ops.Request w) {
        String id = Long.toString((long) w.id);
        for (int attempt = 0; attempt < 2; attempt++) {
            String token = accessToken(attempt > 0);
            if (token == null) {
                signedOut();
                return "{\"id\":" + id + ",\"status\":401}";
            }
            HttpURLConnection c = null;
            try {
                c = open(fence.appOrigin + w.path, "POST", token);
                c.setDoOutput(true);
                c.setRequestProperty("Content-Type", "application/json");
                try (OutputStream o = c.getOutputStream()) {
                    o.write(w.body.getBytes(StandardCharsets.UTF_8));
                }
                int code = c.getResponseCode();
                if (expired(c, code)) continue;
                keepScreenCookie(c);
                String body = readLimited(code >= 400 ? c.getErrorStream() : c.getInputStream());
                return "{\"id\":" + id + ",\"status\":" + code + (body.isEmpty() ? "" : ",\"body\":" + Json.quote(body)) + "}";
            } catch (IOException e) {
                return "{\"id\":" + id + ",\"status\":502}";
            } finally {
                if (c != null) c.disconnect();
            }
        }
        signedOut();
        return "{\"id\":" + id + ",\"status\":401}";
    }

    private HttpURLConnection open(String url, String method, String token) throws IOException {
        if (!fence.ownsApp(url)) throw new IOException("not the app");
        HttpURLConnection c = (HttpURLConnection) new URL(url).openConnection();
        c.setInstanceFollowRedirects(false);
        c.setUseCaches(false);
        c.setConnectTimeout(TIMEOUT_MS);
        c.setReadTimeout(TIMEOUT_MS);
        c.setRequestMethod(method);
        c.setRequestProperty("Authorization", "Bearer " + token);
        Vault.Secrets s = vault.get();
        if (s != null && s.screen != null) c.setRequestProperty("Cookie", SCREEN_COOKIE + "=" + s.screen);
        return c;
    }

    private boolean expired(HttpURLConnection c, int code) {
        if (code == 401) return true;
        if (code >= 300 && code < 400) {
            String loc = c.getHeaderField("Location");
            return loc != null && fence.authority(resolve(c.getURL().toString(), loc));
        }
        return false;
    }

    private void keepScreenCookie(HttpURLConnection c) {
        List<String> set = c.getHeaderFields().get("Set-Cookie");
        if (set == null) return;
        for (String h : set) {
            String first = h.split(";", 2)[0].trim();
            if (!first.startsWith(SCREEN_COOKIE + "=")) continue;
            String value = first.substring(SCREEN_COOKIE.length() + 1);
            synchronized (tokenLock) {
                Vault.Secrets s = vault.get();
                if (s != null) vault.put(new Vault.Secrets(s.refresh, value.isEmpty() ? null : value));
            }
        }
    }

    private static String resolve(String base, String location) {
        try {
            return URI.create(base).resolve(location).toString();
        } catch (IllegalArgumentException e) {
            return "";
        }
    }

    private WebResourceResponse fetch(String url, String method, Map<String, String> headers, boolean mainFrame, boolean mayRetry) {
        String token = accessToken(false);
        if (token == null) {
            signedOut();
            return refused(401);
        }
        HttpURLConnection c = null;
        try {
            c = open(url, method, token);
            for (Map.Entry<String, String> h : headers.entrySet()) {
                String k = h.getKey().toLowerCase(Locale.ROOT);
                if (k.equals("cookie") || k.equals("authorization") || k.equals("host") || k.equals("accept-encoding")
                    || k.startsWith("if-") || k.startsWith("proxy-")) continue;
                c.setRequestProperty(h.getKey(), h.getValue());
            }
            int code = c.getResponseCode();
            if (expired(c, code)) {
                c.disconnect();
                c = null;
                if (mayRetry && accessToken(true) != null) return fetch(url, method, headers, mainFrame, false);
                signedOut();
                return refused(401);
            }
            if (code >= 300 && code < 400) {
                String to = resolve(url, c.getHeaderField("Location"));
                if (mainFrame && fence.ownsApp(to) && fence.opens(to)) return hop(to);
                return refused(502);
            }
            Map<String, String> out = new HashMap<>();
            for (Map.Entry<String, List<String>> h : c.getHeaderFields().entrySet()) {
                String k = h.getKey();
                if (k == null || h.getValue().isEmpty()) continue;
                String lk = k.toLowerCase(Locale.ROOT);
                if (lk.equals("set-cookie") || lk.equals("content-encoding") || lk.equals("content-length")
                    || lk.equals("transfer-encoding") || lk.equals("connection")) continue;
                out.put(k, String.join(", ", h.getValue()));
            }
            String type = c.getContentType();
            String mime = "application/octet-stream", charset = null;
            if (type != null) {
                String[] parts = type.split(";");
                mime = parts[0].trim();
                for (int i = 1; i < parts.length; i++) {
                    String p = parts[i].trim();
                    if (p.toLowerCase(Locale.ROOT).startsWith("charset=")) charset = p.substring(8).replace("\"", "");
                }
            }
            InputStream body = code >= 400 ? c.getErrorStream() : c.getInputStream();
            if (body == null || method.equals("HEAD")) body = new ByteArrayInputStream(new byte[0]);
            String reason = c.getResponseMessage();
            final HttpURLConnection conn = c;
            c = null;
            InputStream stream = new ClosingStream(body, conn);
            return new WebResourceResponse(mime, charset, code, reason == null || reason.isEmpty() ? "Status" : reason, out, stream);
        } catch (IOException e) {
            return refused(502);
        } finally {
            if (c != null) c.disconnect();
        }
    }

    private static final class ClosingStream extends InputStream {
        private final InputStream in;
        private final HttpURLConnection conn;

        ClosingStream(InputStream in, HttpURLConnection conn) {
            this.in = in;
            this.conn = conn;
        }

        @Override
        public int read() throws IOException {
            return in.read();
        }

        @Override
        public int read(byte[] b, int off, int len) throws IOException {
            return in.read(b, off, len);
        }

        @Override
        public int available() throws IOException {
            return in.available();
        }

        @Override
        public void close() throws IOException {
            try {
                in.close();
            } finally {
                conn.disconnect();
            }
        }
    }

    private static WebResourceResponse hop(String to) {
        String html = "<!doctype html><meta http-equiv=\"refresh\" content=\"0;url=" + to.replace("&", "&amp;").replace("\"", "&quot;") + "\">";
        return new WebResourceResponse("text/html", "utf-8", 200, "OK", new HashMap<String, String>(),
            new ByteArrayInputStream(html.getBytes(StandardCharsets.UTF_8)));
    }

    private static String readLimited(InputStream in) throws IOException {
        if (in == null) return "";
        try (InputStream s = in) {
            ByteArrayOutputStream out = new ByteArrayOutputStream();
            byte[] buf = new byte[8192];
            for (int n; (n = s.read(buf)) > 0; ) {
                out.write(buf, 0, n);
                if (out.size() > MAX_REPLY) return "";
            }
            return new String(out.toByteArray(), StandardCharsets.UTF_8);
        }
    }

    private static WebResourceResponse refused(int code) {
        WebResourceResponse r = new WebResourceResponse("text/plain", "utf-8", new ByteArrayInputStream(new byte[0]));
        r.setStatusCodeAndReasonPhrase(code, "Refused");
        return r;
    }

    private void confirmSignOut() {
        new AlertDialog.Builder(this)
            .setTitle("Sign this TV out?")
            .setMessage("It will show a new code to sign in again.")
            .setPositiveButton("Sign out", (d, w) -> writes.execute(() -> {
                Vault.Secrets s = vault.get();
                if (s != null) auth.revoke(s.refresh);
                signedOut();
            }))
            .setNegativeButton("Cancel", null)
            .show();
    }

    @Override
    public boolean dispatchKeyEvent(KeyEvent e) {
        if (web != null && e.getKeyCode() == KeyEvent.KEYCODE_MENU) {
            if (e.getAction() == KeyEvent.ACTION_DOWN && e.getRepeatCount() > 0 && !menuHeld) {
                menuHeld = true;
                confirmSignOut();
                return true;
            }
            if (e.getAction() == KeyEvent.ACTION_UP && menuHeld) {
                menuHeld = false;
                return true;
            }
        }
        if (e.getKeyCode() == KeyEvent.KEYCODE_BACK) {
            if (web != null && e.getAction() == KeyEvent.ACTION_UP) {
                if (web.canGoBack()) web.goBack();
                else backToPage();
            }
            return true;
        }
        return super.dispatchKeyEvent(e);
    }

    @Override
    protected void onDestroy() {
        signIn = null;
        closePorts();
        writes.shutdownNow();
        if (web != null) web.destroy();
        super.onDestroy();
    }
}
