package tv.aguatv.player;

import android.app.Activity;
import android.content.Intent;
import android.graphics.drawable.ColorDrawable;
import android.media.MediaPlayer;
import android.net.Uri;
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
import android.widget.ProgressBar;
import android.widget.TextView;
import android.widget.VideoView;

public final class Player extends Activity {
    static final String EXTRA_TITLE = "title";
    private static final int MATCH = ViewGroup.LayoutParams.MATCH_PARENT;
    private static final int WRAP = ViewGroup.LayoutParams.WRAP_CONTENT;
    private static final int INK = 0xFFF0DFC8, MUTED = 0xFFA5887B, GOLD = 0xFFD9A94E, VEIL = 0xCC1A0D0F;
    private static final long OSD_MS = 3000, TICK_MS = 500;
    private static final int STEP_MS = 10000, JUMP_MS = 30000;

    private final Handler ui = new Handler(Looper.getMainLooper());
    private VideoView video;
    private ProgressBar spinner;
    private TextView title, clock, status;
    private View osd;
    private int resumeAt;
    private boolean prepared;

    private final Runnable hideOsd = () -> osd.setVisibility(View.GONE);
    private final Runnable tick = new Runnable() {
        @Override
        public void run() {
            clock.setText(time(video.getCurrentPosition()) + "  /  " + time(video.getDuration())
                    + (video.isPlaying() ? "" : "   paused"));
            if (osd.getVisibility() == View.VISIBLE) ui.postDelayed(this, TICK_MS);
        }
    };

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        getWindow().setBackgroundDrawable(new ColorDrawable(0xFF000000));
        FrameLayout root = new FrameLayout(this);

        video = new VideoView(this);
        root.addView(video, new FrameLayout.LayoutParams(MATCH, MATCH, Gravity.CENTER));

        spinner = new ProgressBar(this);
        root.addView(spinner, new FrameLayout.LayoutParams(WRAP, WRAP, Gravity.CENTER));

        status = text(22, MUTED);
        status.setVisibility(View.GONE);
        root.addView(status, new FrameLayout.LayoutParams(WRAP, WRAP, Gravity.CENTER));

        FrameLayout bar = new FrameLayout(this);
        bar.setBackground(new ColorDrawable(VEIL));
        bar.setPadding(dp(48), dp(20), dp(48), dp(24));
        title = text(22, INK);
        bar.addView(title, new FrameLayout.LayoutParams(WRAP, WRAP, Gravity.START | Gravity.CENTER_VERTICAL));
        clock = text(20, GOLD);
        bar.addView(clock, new FrameLayout.LayoutParams(WRAP, WRAP, Gravity.END | Gravity.CENTER_VERTICAL));
        osd = bar;
        root.addView(bar, new FrameLayout.LayoutParams(MATCH, WRAP, Gravity.BOTTOM));
        setContentView(root);

        video.setOnPreparedListener(this::onPrepared);
        video.setOnInfoListener((mp, what, extra) -> {
            if (what == MediaPlayer.MEDIA_INFO_BUFFERING_START) spinner.setVisibility(View.VISIBLE);
            if (what == MediaPlayer.MEDIA_INFO_BUFFERING_END || what == MediaPlayer.MEDIA_INFO_VIDEO_RENDERING_START) {
                spinner.setVisibility(View.GONE);
            }
            return false;
        });
        video.setOnErrorListener((mp, what, extra) -> {
            spinner.setVisibility(View.GONE);
            status.setText("Cannot play this (" + what + ", " + extra + ")");
            status.setVisibility(View.VISIBLE);
            screenOn(false);
            return true;
        });
        video.setOnCompletionListener(mp -> finish());
        open(getIntent());
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        setIntent(intent);
        resumeAt = 0;
        open(intent);
    }

    private void open(Intent intent) {
        Uri uri = intent.getData();
        if (uri == null || !"https".equals(uri.getScheme())) {
            finish();
            return;
        }
        prepared = false;
        String name = intent.getStringExtra(EXTRA_TITLE);
        if (name == null) name = uri.getLastPathSegment();
        title.setText(name == null ? "" : name);
        status.setVisibility(View.GONE);
        spinner.setVisibility(View.VISIBLE);
        video.setVideoURI(uri);
        showOsd();
    }

    private void onPrepared(MediaPlayer mp) {
        prepared = true;
        spinner.setVisibility(View.GONE);
        if (resumeAt > 0) video.seekTo(resumeAt);
        video.start();
        screenOn(true);
        showOsd();
    }

    @Override
    protected void onPause() {
        super.onPause();
        if (prepared) {
            resumeAt = video.getCurrentPosition();
            video.pause();
        }
        screenOn(false);
    }

    @Override
    protected void onResume() {
        super.onResume();
        if (prepared) {
            video.seekTo(resumeAt);
            video.start();
            screenOn(true);
        }
    }

    @Override
    protected void onDestroy() {
        ui.removeCallbacksAndMessages(null);
        video.stopPlayback();
        super.onDestroy();
    }

    @Override
    public boolean dispatchKeyEvent(KeyEvent e) {
        int code = e.getKeyCode();
        if (code == KeyEvent.KEYCODE_BACK) {
            if (e.getAction() == KeyEvent.ACTION_UP) finish();
            return true;
        }
        if (e.getAction() != KeyEvent.ACTION_DOWN) return isOurs(code) || super.dispatchKeyEvent(e);
        switch (code) {
            case KeyEvent.KEYCODE_DPAD_CENTER:
            case KeyEvent.KEYCODE_ENTER:
            case KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE:
                toggle();
                return true;
            case KeyEvent.KEYCODE_MEDIA_PLAY:
                if (prepared && !video.isPlaying()) toggle();
                return true;
            case KeyEvent.KEYCODE_MEDIA_PAUSE:
                if (prepared && video.isPlaying()) toggle();
                return true;
            case KeyEvent.KEYCODE_DPAD_LEFT:
                seek(-STEP_MS);
                return true;
            case KeyEvent.KEYCODE_DPAD_RIGHT:
                seek(STEP_MS);
                return true;
            case KeyEvent.KEYCODE_MEDIA_REWIND:
                seek(-JUMP_MS);
                return true;
            case KeyEvent.KEYCODE_MEDIA_FAST_FORWARD:
                seek(JUMP_MS);
                return true;
            case KeyEvent.KEYCODE_DPAD_UP:
            case KeyEvent.KEYCODE_DPAD_DOWN:
            case KeyEvent.KEYCODE_INFO:
            case KeyEvent.KEYCODE_MENU:
                showOsd();
                return true;
            default:
                return super.dispatchKeyEvent(e);
        }
    }

    private static boolean isOurs(int code) {
        switch (code) {
            case KeyEvent.KEYCODE_DPAD_CENTER:
            case KeyEvent.KEYCODE_ENTER:
            case KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE:
            case KeyEvent.KEYCODE_MEDIA_PLAY:
            case KeyEvent.KEYCODE_MEDIA_PAUSE:
            case KeyEvent.KEYCODE_DPAD_LEFT:
            case KeyEvent.KEYCODE_DPAD_RIGHT:
            case KeyEvent.KEYCODE_MEDIA_REWIND:
            case KeyEvent.KEYCODE_MEDIA_FAST_FORWARD:
            case KeyEvent.KEYCODE_DPAD_UP:
            case KeyEvent.KEYCODE_DPAD_DOWN:
            case KeyEvent.KEYCODE_INFO:
            case KeyEvent.KEYCODE_MENU:
                return true;
            default:
                return false;
        }
    }

    private void toggle() {
        if (!prepared) return;
        if (video.isPlaying()) {
            video.pause();
            screenOn(false);
        } else {
            video.start();
            screenOn(true);
        }
        showOsd();
    }

    private void seek(int delta) {
        if (!prepared) return;
        int to = Math.max(0, video.getCurrentPosition() + delta);
        int end = video.getDuration();
        if (end > 0) to = Math.min(to, end - 1000);
        video.seekTo(to);
        showOsd();
    }

    private void showOsd() {
        osd.setVisibility(View.VISIBLE);
        ui.removeCallbacks(tick);
        ui.post(tick);
        ui.removeCallbacks(hideOsd);
        if (prepared && video.isPlaying()) ui.postDelayed(hideOsd, OSD_MS);
    }

    private void screenOn(boolean on) {
        if (on) getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        else getWindow().clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
    }

    private static String time(int ms) {
        if (ms < 0) return "--:--";
        int s = ms / 1000, h = s / 3600, m = s / 60 % 60;
        s %= 60;
        return h > 0 ? String.format("%d:%02d:%02d", h, m, s) : String.format("%d:%02d", m, s);
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
