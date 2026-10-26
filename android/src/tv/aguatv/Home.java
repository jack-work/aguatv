package tv.aguatv;

import android.app.Activity;
import android.content.ActivityNotFoundException;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.IntentFilter;
import android.media.tv.TvInputManager;
import android.net.Uri;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.provider.Settings;
import android.view.KeyEvent;
import android.webkit.RenderProcessGoneDetail;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.FrameLayout;
import android.widget.Toast;

import java.io.ByteArrayInputStream;
import java.io.IOException;
import java.io.InputStream;
import java.util.Collections;

public final class Home extends Activity {
    static final String ORIGIN = "https://aguatv.invalid";
    private static final int STAGE = 0xFF1A0D0F;

    private final Handler ui = new Handler(Looper.getMainLooper());
    private FrameLayout root;
    private WebView web;
    private boolean ready;
    private TvInputManager tv;

    private final BroadcastReceiver packages = new BroadcastReceiver() {
        @Override
        public void onReceive(Context c, Intent i) {
            call("refresh");
        }
    };

    private final TvInputManager.TvInputCallback inputs = new TvInputManager.TvInputCallback() {
        @Override
        public void onInputStateChanged(String inputId, int state) {
            call("refresh");
        }

        @Override
        public void onInputAdded(String inputId) {
            call("refresh");
        }

        @Override
        public void onInputRemoved(String inputId) {
            call("refresh");
        }
    };

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        root = new FrameLayout(this);
        root.setBackgroundColor(STAGE);
        setContentView(root);
        mount();

        IntentFilter f = new IntentFilter();
        f.addAction(Intent.ACTION_PACKAGE_ADDED);
        f.addAction(Intent.ACTION_PACKAGE_REMOVED);
        f.addAction(Intent.ACTION_PACKAGE_CHANGED);
        f.addDataScheme("package");
        registerReceiver(packages, f);

        tv = (TvInputManager) getSystemService(Context.TV_INPUT_SERVICE);
        if (tv != null) tv.registerCallback(inputs, ui);
    }

    private void mount() {
        ready = false;
        web = new WebView(this);
        web.setBackgroundColor(STAGE);
        web.setFocusable(true);
        web.setFocusableInTouchMode(true);
        WebSettings s = web.getSettings();
        s.setJavaScriptEnabled(true);
        s.setDomStorageEnabled(true);
        s.setAllowFileAccess(false);
        s.setAllowContentAccess(false);
        s.setCacheMode(WebSettings.LOAD_NO_CACHE);
        s.setMediaPlaybackRequiresUserGesture(true);
        web.addJavascriptInterface(new Bridge(this), "host");
        web.setWebViewClient(new Client());
        web.setRendererPriorityPolicy(WebView.RENDERER_PRIORITY_IMPORTANT, true);
        root.addView(web, new FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT));
        web.loadUrl(ORIGIN + "/index.html");
        web.requestFocus();
    }

    private final class Client extends WebViewClient {
        @Override
        public WebResourceResponse shouldInterceptRequest(WebView view, WebResourceRequest req) {
            Uri u = req.getUrl();
            if (!ORIGIN.equals(u.getScheme() + "://" + u.getHost())) return refuse();
            String path = u.getPath() == null ? "/" : u.getPath();
            if (path.startsWith("/art/")) {
                byte[] png = Shelf.art(Home.this, path.substring(5));
                return png == null ? missing() : answer("image/png", new ByteArrayInputStream(png));
            }
            if (path.equals("/")) path = "/index.html";
            if (path.contains("..")) return refuse();
            try {
                return answer(mime(path), getAssets().open("web" + path));
            } catch (IOException e) {
                return missing();
            }
        }

        @Override
        public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest req) {
            return !ORIGIN.equals(req.getUrl().getScheme() + "://" + req.getUrl().getHost());
        }

        @Override
        public boolean onRenderProcessGone(WebView view, RenderProcessGoneDetail detail) {
            if (view != web) return true;
            root.removeView(web);
            web.destroy();
            web = null;
            if (!isFinishing()) mount();
            return true;
        }
    }

    private static WebResourceResponse answer(String mime, InputStream body) {
        WebResourceResponse r = new WebResourceResponse(mime, "utf-8", body);
        r.setResponseHeaders(Collections.singletonMap("Cache-Control", "no-store"));
        return r;
    }

    private static WebResourceResponse missing() {
        return new WebResourceResponse("text/plain", "utf-8", 404, "Not Found",
                Collections.<String, String>emptyMap(), new ByteArrayInputStream(new byte[0]));
    }

    private static WebResourceResponse refuse() {
        return new WebResourceResponse("text/plain", "utf-8", 403, "Forbidden",
                Collections.<String, String>emptyMap(), new ByteArrayInputStream(new byte[0]));
    }

    private static String mime(String path) {
        if (path.endsWith(".html")) return "text/html";
        if (path.endsWith(".css")) return "text/css";
        if (path.endsWith(".js")) return "text/javascript";
        if (path.endsWith(".svg")) return "image/svg+xml";
        if (path.endsWith(".png")) return "image/png";
        if (path.endsWith(".woff2")) return "font/woff2";
        if (path.endsWith(".json")) return "application/json";
        return "application/octet-stream";
    }

    void pageReady() {
        ui.post(() -> {
            ready = true;
            Guard.shown();
        });
    }

    void openApp(String pkg) {
        ui.post(() -> {
            Shelf.App a = Shelf.apps(this).get(pkg);
            if (a == null) return;
            launch(new Intent(a.intent));
        });
    }

    void openInput(String inputId) {
        ui.post(() -> {
            Intent i = Input.intent(this, inputId, Shelf.inputLabel(this, inputId));
            if (i.getComponent() != null && getPackageName().equals(i.getComponent().getPackageName())) {
                startActivity(i);
            } else {
                launch(i);
            }
        });
    }

    void openSettings() {
        ui.post(() -> {
            Intent tvSettings = new Intent(Intent.ACTION_MAIN)
                    .addCategory("android.intent.category.LEANBACK_SETTINGS");
            boolean found = tvSettings.resolveActivity(getPackageManager()) != null;
            launch(found ? tvSettings : new Intent(Settings.ACTION_SETTINGS));
        });
    }

    private void launch(Intent i) {
        try {
            startActivity(i.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_RESET_TASK_IF_NEEDED));
        } catch (ActivityNotFoundException | SecurityException e) {
            Toast.makeText(this, "Could not open that", Toast.LENGTH_SHORT).show();
        }
    }

    private void call(String fn) {
        if (web == null) return;
        web.evaluateJavascript("window.aguatv && aguatv." + fn + " && aguatv." + fn + "()", null);
    }

    @Override
    protected void onResume() {
        super.onResume();
        Guard.arrived(this);
        if (web != null) {
            web.setRendererPriorityPolicy(WebView.RENDERER_PRIORITY_IMPORTANT, true);
            web.onResume();
            web.resumeTimers();
            web.requestFocus();
        }
        call("resume");
    }

    @Override
    public void onEnterAnimationComplete() {
        if (ready) Guard.shown();
    }

    @Override
    protected void onPause() {
        super.onPause();
        if (web != null) {
            call("pause");
            web.onPause();
            web.pauseTimers();
            web.setRendererPriorityPolicy(WebView.RENDERER_PRIORITY_WAIVED, true);
        }
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        call("home");
    }

    @Override
    public boolean dispatchKeyEvent(KeyEvent e) {
        int code = e.getKeyCode();
        if (code == KeyEvent.KEYCODE_BACK || code == KeyEvent.KEYCODE_MENU) {
            if (e.getAction() == KeyEvent.ACTION_UP) call(code == KeyEvent.KEYCODE_BACK ? "back" : "menu");
            return true;
        }
        return super.dispatchKeyEvent(e);
    }

    @Override
    public void onTrimMemory(int level) {
        super.onTrimMemory(level);
        if (level < TRIM_MEMORY_UI_HIDDEN || level >= TRIM_MEMORY_COMPLETE) return;
        try {
            Class<?> wmg = Class.forName("android.view.WindowManagerGlobal");
            Object global = wmg.getMethod("getInstance").invoke(null);
            wmg.getMethod("trimMemory", int.class).invoke(global, TRIM_MEMORY_COMPLETE);
        } catch (ReflectiveOperationException | RuntimeException ignored) {
        }
    }

    @Override
    protected void onDestroy() {
        unregisterReceiver(packages);
        if (tv != null) tv.unregisterCallback(inputs);
        if (web != null) {
            root.removeView(web);
            web.destroy();
        }
        super.onDestroy();
    }
}
