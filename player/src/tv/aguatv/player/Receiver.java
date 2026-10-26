package tv.aguatv.player;

import android.app.Activity;
import android.content.Intent;
import android.content.SharedPreferences;
import android.graphics.Color;
import android.net.Uri;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.provider.Settings;
import android.text.TextUtils;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.KeyEvent;
import android.view.WindowManager;
import android.webkit.CookieManager;
import android.webkit.JavascriptInterface;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.FrameLayout;
import android.widget.TextView;

public final class Receiver extends Activity {
    static final String EXTRA_HUB = "hub";
    private static final String PREFS = "receiver";
    private static final int STAGE = 0xFF1A0D0F, INK = 0xFFF0DFC8;

    private final Handler ui = new Handler(Looper.getMainLooper());
    private WebView web;
    private String hub;

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        SharedPreferences prefs = getSharedPreferences(PREFS, MODE_PRIVATE);
        String given = getIntent().getStringExtra(EXTRA_HUB);
        if (acceptable(given)) {
            prefs.edit().putString(EXTRA_HUB, trim(given)).apply();
        }
        hub = prefs.getString(EXTRA_HUB, null);

        FrameLayout root = new FrameLayout(this);
        root.setBackgroundColor(STAGE);
        setContentView(root);
        if (hub == null) {
            TextView t = new TextView(this);
            t.setTextColor(INK);
            t.setTextSize(TypedValue.COMPLEX_UNIT_SP, 22);
            t.setGravity(Gravity.CENTER);
            t.setText("No hub is set. From a computer: aguatv receiver <hub URL>");
            root.addView(t, new FrameLayout.LayoutParams(-1, -1));
            return;
        }

        CookieManager.getInstance().setAcceptCookie(true);
        web = new WebView(this);
        web.setBackgroundColor(Color.BLACK);
        WebSettings s = web.getSettings();
        s.setJavaScriptEnabled(true);
        s.setDomStorageEnabled(true);
        s.setMediaPlaybackRequiresUserGesture(false);
        s.setAllowFileAccess(false);
        s.setAllowContentAccess(false);
        web.addJavascriptInterface(new Bridge(), "receiverHost");
        web.setWebViewClient(new WebViewClient() {
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest req) {
                return !req.getUrl().toString().startsWith(hub + "/");
            }
        });
        root.addView(web, new FrameLayout.LayoutParams(-1, -1));
        web.loadUrl(hub + "/receiver?kind=tv&name=" + Uri.encode(deviceName()));
        web.requestFocus();
    }

    private static boolean acceptable(String url) {
        return url != null && (url.startsWith("https://") || url.startsWith("http://localhost:"));
    }

    private static String trim(String url) {
        return url.endsWith("/") ? url.substring(0, url.length() - 1) : url;
    }

    private String deviceName() {
        String name = Settings.Global.getString(getContentResolver(), "device_name");
        return TextUtils.isEmpty(name) ? "TV" : name;
    }

    private final class Bridge {
        @JavascriptInterface
        public void keepAwake(boolean on) {
            ui.post(() -> {
                if (on) getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
                else getWindow().clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
            });
        }
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        String given = intent.getStringExtra(EXTRA_HUB);
        if (acceptable(given) && !trim(given).equals(hub)) {
            setIntent(intent);
            recreate();
        }
    }

    @Override
    public boolean dispatchKeyEvent(KeyEvent e) {
        if (web != null && e.getKeyCode() == KeyEvent.KEYCODE_BACK) {
            if (e.getAction() == KeyEvent.ACTION_UP) {
                web.evaluateJavascript("window.aguatvReceiver ? window.aguatvReceiver.back() : false", v -> {
                    if (!"true".equals(v)) finish();
                });
            }
            return true;
        }
        return super.dispatchKeyEvent(e);
    }

    @Override
    protected void onPause() {
        super.onPause();
        CookieManager.getInstance().flush();
    }

    @Override
    protected void onDestroy() {
        if (web != null) web.destroy();
        super.onDestroy();
    }
}
