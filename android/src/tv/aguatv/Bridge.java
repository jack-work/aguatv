package tv.aguatv;

import android.webkit.JavascriptInterface;

final class Bridge {
    private final Home home;

    Bridge(Home home) {
        this.home = home;
    }

    @JavascriptInterface
    public String apps() {
        return Shelf.appsJson(home);
    }

    @JavascriptInterface
    public String inputs() {
        return Shelf.inputsJson(home);
    }

    @JavascriptInterface
    public void openApp(String pkg) {
        home.openApp(pkg);
    }

    @JavascriptInterface
    public void openInput(String inputId) {
        home.openInput(inputId);
    }

    @JavascriptInterface
    public void openSettings() {
        home.openSettings();
    }

    @JavascriptInterface
    public void openAmazon() {
        home.openAmazon();
    }

    @JavascriptInterface
    public void ready() {
        home.pageReady();
    }
}
