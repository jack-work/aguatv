package tv.aguatv;

import android.app.Activity;
import android.content.Context;
import android.content.Intent;
import android.content.pm.ResolveInfo;
import android.graphics.Color;
import android.graphics.drawable.ColorDrawable;
import android.graphics.drawable.GradientDrawable;
import android.media.AudioManager;
import android.media.tv.TvContract;
import android.media.tv.TvInputManager;
import android.media.tv.TvView;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.KeyEvent;
import android.view.View;
import android.view.ViewGroup;
import android.view.WindowManager;
import android.widget.FrameLayout;
import android.widget.TextView;

import java.util.List;

public final class Input extends Activity {
    private static final int MATCH = ViewGroup.LayoutParams.MATCH_PARENT;
    private static final int WRAP = ViewGroup.LayoutParams.WRAP_CONTENT;
    private static final String EXTRA_ID = "input", EXTRA_NAME = "name";
    private static final long NAME_MS = 2500, SIGNAL_MS = 6000;
    private static final String FAILED = "Could not open input", NO_SIGNAL = "No signal";
    private static final String AMAZON_LIVE_TV = "com.amazon.tv.livetv";
    private static final int INK = 0xFFF0DFC8, MUTED = 0xFFA5887B, VEIL = 0xB31A0D0F;

    private final Handler ui = new Handler(Looper.getMainLooper());
    private final AudioManager.OnAudioFocusChangeListener focus = change -> { };
    private TvView view;
    private TextView name, status;
    private String id;
    private final Runnable hideName = () -> name.animate().alpha(0f).setDuration(250)
            .withEndAction(() -> name.setVisibility(View.GONE)).start();
    private final Runnable noSignal = () -> status(NO_SIGNAL);

    private final TvView.TvInputCallback watch = new TvView.TvInputCallback() {
        @Override
        public void onConnectionFailed(String inputId) {
            status(FAILED);
        }

        @Override
        public void onDisconnected(String inputId) {
            status(FAILED);
        }

        @Override
        public void onVideoAvailable(String inputId) {
            status(null);
        }

        @Override
        public void onVideoUnavailable(String inputId, int reason) {
            boolean pending = reason == TvInputManager.VIDEO_UNAVAILABLE_REASON_TUNING
                    || reason == TvInputManager.VIDEO_UNAVAILABLE_REASON_BUFFERING;
            if (!pending) status(NO_SIGNAL);
        }
    };

    static Intent intent(Context c, String inputId, CharSequence label) {
        Intent passthrough = passthrough(inputId);
        List<ResolveInfo> apps = c.getPackageManager().queryIntentActivities(passthrough, 0);
        if (apps.size() == 1 && !isLiveTv(apps.get(0))) return passthrough;
        return new Intent(c, Input.class).putExtra(EXTRA_ID, inputId).putExtra(EXTRA_NAME, label);
    }

    static Intent passthrough(String inputId) {
        return new Intent(Intent.ACTION_VIEW, TvContract.buildChannelUriForPassthroughInput(inputId));
    }

    private static boolean isLiveTv(ResolveInfo app) {
        return app.activityInfo != null && AMAZON_LIVE_TV.equals(app.activityInfo.packageName);
    }

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        id = getIntent().getStringExtra(EXTRA_ID);
        getWindow().setBackgroundDrawable(new ColorDrawable(Color.TRANSPARENT));

        FrameLayout root = new FrameLayout(this);
        if (getSystemService(Context.TV_INPUT_SERVICE) != null) {
            view = new TvView(this);
            view.setCallback(watch);
            view.setFocusable(true);
            root.addView(view, new FrameLayout.LayoutParams(MATCH, MATCH));
        }

        status = text(22, MUTED);
        status.setVisibility(View.GONE);
        root.addView(status, new FrameLayout.LayoutParams(WRAP, WRAP, Gravity.CENTER));

        name = text(20, INK);
        name.setText(getIntent().getCharSequenceExtra(EXTRA_NAME));
        name.setPadding(dp(22), dp(10), dp(22), dp(10));
        GradientDrawable plate = new GradientDrawable();
        plate.setColor(VEIL);
        name.setBackground(plate);
        FrameLayout.LayoutParams lp = new FrameLayout.LayoutParams(WRAP, WRAP, Gravity.TOP | Gravity.START);
        lp.setMargins(dp(56), dp(40), 0, 0);
        root.addView(name, lp);
        setContentView(root);
    }

    @Override
    @SuppressWarnings("deprecation")
    protected void onStart() {
        super.onStart();
        showName();
        if (view == null || id == null) {
            status(FAILED);
            return;
        }
        ((AudioManager) getSystemService(Context.AUDIO_SERVICE))
                .requestAudioFocus(focus, AudioManager.STREAM_MUSIC, AudioManager.AUDIOFOCUS_GAIN);
        view.setStreamVolume(1f);
        view.tune(id, TvContract.buildChannelUriForPassthroughInput(id));
        view.requestFocus();
        ui.postDelayed(noSignal, SIGNAL_MS);
    }

    @Override
    @SuppressWarnings("deprecation")
    protected void onStop() {
        ui.removeCallbacksAndMessages(null);
        getWindow().clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        if (view != null) {
            view.reset();
            ((AudioManager) getSystemService(Context.AUDIO_SERVICE)).abandonAudioFocus(focus);
        }
        super.onStop();
    }

    @Override
    public boolean dispatchKeyEvent(KeyEvent e) {
        switch (e.getKeyCode()) {
            case KeyEvent.KEYCODE_BACK:
                if (e.getAction() == KeyEvent.ACTION_UP) finish();
                return true;
            case KeyEvent.KEYCODE_MENU:
            case KeyEvent.KEYCODE_INFO:
                if (e.getAction() == KeyEvent.ACTION_DOWN) showName();
                return true;
            default:
                return super.dispatchKeyEvent(e);
        }
    }

    private void showName() {
        ui.removeCallbacks(hideName);
        name.animate().cancel();
        name.setAlpha(1f);
        name.setVisibility(View.VISIBLE);
        ui.postDelayed(hideName, NAME_MS);
    }

    private void status(String text) {
        if (text == null) getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        else getWindow().clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        ui.removeCallbacks(noSignal);
        status.setText(text);
        status.setVisibility(text == null ? View.GONE : View.VISIBLE);
    }

    private TextView text(float sp, int color) {
        TextView t = new TextView(this);
        t.setTextSize(TypedValue.COMPLEX_UNIT_SP, sp);
        t.setTextColor(color);
        return t;
    }

    private int dp(float v) {
        return Math.round(v * getResources().getDisplayMetrics().density);
    }
}
