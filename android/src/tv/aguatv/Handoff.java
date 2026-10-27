package tv.aguatv;

import android.content.ActivityNotFoundException;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageManager;

final class Handoff {
    static final String PLAYER = "tv.aguatv.player";
    private static final String RECEIVER = "tv.aguatv.player.Receiver";

    private Handoff() {}

    static Intent receiver() {
        Intent i = new Intent(Intent.ACTION_MAIN)
                .setComponent(new ComponentName(PLAYER, RECEIVER))
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_RESET_TASK_IF_NEEDED);
        if (i.getExtras() != null || i.getData() != null) throw new IllegalStateException("handoff carries data");
        return i;
    }

    static boolean installed(Context c) {
        try {
            c.getPackageManager().getActivityInfo(new ComponentName(PLAYER, RECEIVER), 0);
            return true;
        } catch (PackageManager.NameNotFoundException e) {
            return false;
        }
    }

    static boolean go(Context c) {
        if (!installed(c)) return false;
        try {
            c.startActivity(receiver());
            return true;
        } catch (ActivityNotFoundException | SecurityException e) {
            return false;
        }
    }
}
