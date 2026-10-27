package tv.aguatv.player;

import android.app.Activity;
import android.content.Intent;
import android.content.SharedPreferences;
import android.graphics.Color;
import android.net.Uri;
import android.net.http.SslError;
import android.os.Bundle;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.KeyEvent;
import android.view.WindowManager;
import android.webkit.CookieManager;
import android.webkit.PermissionRequest;
import android.webkit.SslErrorHandler;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.FrameLayout;
import android.widget.TextView;

import java.io.ByteArrayInputStream;

public final class Receiver extends Activity {
    static final String EXTRA_APP = "app";
    static final String EXTRA_AUTH = "auth";
    static final String EXTRA_ALSO = "also";
    private static final String PREFS = "receiver";
    private static final int STAGE = 0xFF1A0D0F, INK = 0xFFF0DFC8;

    private WebView web;
    private Fence fence;

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

        FrameLayout root = new FrameLayout(this);
        root.setBackgroundColor(STAGE);
        setContentView(root);
        if (fence == null) {
            TextView t = new TextView(this);
            t.setTextColor(INK);
            t.setTextSize(TypedValue.COMPLEX_UNIT_SP, 22);
            t.setGravity(Gravity.CENTER);
            t.setText("No app is set. From a computer: aguatv receiver <app URL> <sign-in URL>");
            root.addView(t, new FrameLayout.LayoutParams(-1, -1));
            return;
        }

        getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        CookieManager cookies = CookieManager.getInstance();
        cookies.setAcceptCookie(true);
        web = new WebView(this);
        cookies.setAcceptThirdPartyCookies(web, false);
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
                if (!req.isForMainFrame()) return !fence.fetches(req.getUrl().toString());
                if (fence.navigates(req.getUrl().toString())) return false;
                view.loadUrl(fence.app);
                return true;
            }

            @Override
            public void doUpdateVisitedHistory(WebView view, String url, boolean reload) {
                if (!fence.navigates(url)) view.loadUrl(fence.app);
            }

            @Override
            public WebResourceResponse shouldInterceptRequest(WebView view, WebResourceRequest req) {
                String u = req.getUrl().toString();
                boolean ok = req.isForMainFrame() ? fence.navigates(u) : fence.fetches(u);
                return ok ? null : refused();
            }

            @Override
            public void onReceivedSslError(WebView view, SslErrorHandler handler, SslError error) {
                handler.cancel();
            }
        });
        root.addView(web, new FrameLayout.LayoutParams(-1, -1));
        web.loadUrl(fence.app);
        web.requestFocus();
    }

    private static WebResourceResponse refused() {
        WebResourceResponse r = new WebResourceResponse("text/plain", "utf-8", new ByteArrayInputStream(new byte[0]));
        r.setStatusCodeAndReasonPhrase(403, "Refused");
        return r;
    }

    @Override
    public boolean dispatchKeyEvent(KeyEvent e) {
        if (web != null && e.getKeyCode() == KeyEvent.KEYCODE_BACK) {
            if (e.getAction() == KeyEvent.ACTION_UP) {
                if (web.canGoBack()) web.goBack();
                else finish();
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
