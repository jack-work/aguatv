package tv.aguatv;

import android.content.Context;
import android.content.Intent;
import android.content.pm.ActivityInfo;
import android.content.pm.ApplicationInfo;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import android.content.pm.ResolveInfo;
import android.graphics.Bitmap;
import android.graphics.Canvas;
import android.graphics.drawable.BitmapDrawable;
import android.graphics.drawable.Drawable;
import android.media.tv.TvInputInfo;
import android.media.tv.TvInputManager;
import android.text.TextUtils;

import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;

import java.io.ByteArrayOutputStream;
import java.text.Collator;
import java.util.ArrayList;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

final class Shelf {
    private static final int ART_MAX = 320;

    private Shelf() {}

    static final class App {
        final String id;
        final String label;
        final boolean leanback;
        final ActivityInfo info;
        final Intent intent;

        App(String id, String label, boolean leanback, ActivityInfo info, Intent intent) {
            this.id = id;
            this.label = label;
            this.leanback = leanback;
            this.info = info;
            this.intent = intent;
        }
    }

    static Map<String, App> apps(Context c) {
        PackageManager pm = c.getPackageManager();
        Map<String, App> found = new LinkedHashMap<>();
        collect(pm, found, Intent.CATEGORY_LEANBACK_LAUNCHER, true, c.getPackageName());
        collect(pm, found, Intent.CATEGORY_LAUNCHER, false, c.getPackageName());
        return found;
    }

    private static void collect(PackageManager pm, Map<String, App> found, String category,
            boolean leanback, String self) {
        Intent query = new Intent(Intent.ACTION_MAIN).addCategory(category);
        for (ResolveInfo ri : pm.queryIntentActivities(query, 0)) {
            ActivityInfo ai = ri.activityInfo;
            if (ai.packageName.equals(self) || found.containsKey(ai.packageName)) continue;
            Intent launch = new Intent(Intent.ACTION_MAIN).addCategory(category)
                    .setClassName(ai.packageName, ai.name);
            found.put(ai.packageName,
                    new App(ai.packageName, ri.loadLabel(pm).toString(), leanback, ai, launch));
        }
    }

    static String appsJson(Context c) {
        List<App> list = new ArrayList<>(apps(c).values());
        Collator collator = Collator.getInstance();
        Collections.sort(list, (a, b) -> collator.compare(a.label, b.label));
        JSONArray out = new JSONArray();
        try {
            for (App a : list) {
                out.put(new JSONObject().put("id", a.id).put("label", a.label)
                        .put("leanback", a.leanback));
            }
        } catch (JSONException e) {
            throw new IllegalStateException(e);
        }
        return out.toString();
    }

    static String inputsJson(Context c) {
        TvInputManager tv = (TvInputManager) c.getSystemService(Context.TV_INPUT_SERVICE);
        JSONArray out = new JSONArray();
        if (tv == null) return out.toString();
        List<JSONObject> hdmi = new ArrayList<>(), other = new ArrayList<>();
        try {
            for (TvInputInfo in : tv.getTvInputList()) {
                if (!in.isPassthroughInput() || in.isHidden(c)) continue;
                CharSequence label = in.loadCustomLabel(c);
                if (TextUtils.isEmpty(label)) label = in.loadLabel(c);
                JSONObject o = new JSONObject().put("id", in.getId())
                        .put("label", label.toString())
                        .put("hdmi", in.getType() == TvInputInfo.TYPE_HDMI)
                        .put("connected", tv.getInputState(in.getId())
                                != TvInputManager.INPUT_STATE_DISCONNECTED);
                (in.getType() == TvInputInfo.TYPE_HDMI ? hdmi : other).add(o);
            }
            Collator collator = Collator.getInstance();
            Collections.sort(hdmi, (a, b) -> collator.compare(a.optString("label"), b.optString("label")));
            Collections.sort(other, (a, b) -> collator.compare(a.optString("label"), b.optString("label")));
        } catch (JSONException e) {
            throw new IllegalStateException(e);
        }
        for (JSONObject o : hdmi) out.put(o);
        for (JSONObject o : other) out.put(o);
        return out.toString();
    }

    static String appDetails(Context c, String pkg) {
        App a = apps(c).get(pkg);
        if (a == null) return "{}";
        PackageManager pm = c.getPackageManager();
        try {
            PackageInfo info = pm.getPackageInfo(pkg, 0);
            ApplicationInfo ai = info.applicationInfo;
            return new JSONObject()
                    .put("kind", "app")
                    .put("id", pkg)
                    .put("label", a.label)
                    .put("version", info.versionName == null ? "" : info.versionName)
                    .put("versionCode", info.getLongVersionCode())
                    .put("installed", info.firstInstallTime)
                    .put("updated", info.lastUpdateTime)
                    .put("system", (ai.flags & ApplicationInfo.FLAG_SYSTEM) != 0)
                    .put("targetSdk", ai.targetSdkVersion)
                    .put("tv", a.leanback)
                    .toString();
        } catch (PackageManager.NameNotFoundException | JSONException e) {
            return "{}";
        }
    }

    static String inputDetails(Context c, String inputId) {
        TvInputManager tv = (TvInputManager) c.getSystemService(Context.TV_INPUT_SERVICE);
        if (tv == null) return "{}";
        TvInputInfo in = tv.getTvInputInfo(inputId);
        if (in == null) return "{}";
        int state = tv.getInputState(inputId);
        String stateName = state == TvInputManager.INPUT_STATE_CONNECTED ? "connected"
                : state == TvInputManager.INPUT_STATE_CONNECTED_STANDBY ? "standby" : "disconnected";
        try {
            return new JSONObject()
                    .put("kind", "input")
                    .put("id", inputId)
                    .put("label", inputLabel(c, inputId).toString())
                    .put("type", typeName(in.getType()))
                    .put("state", stateName)
                    .put("service", in.getServiceInfo().packageName)
                    .toString();
        } catch (JSONException e) {
            return "{}";
        }
    }

    private static String typeName(int type) {
        switch (type) {
            case TvInputInfo.TYPE_HDMI: return "HDMI";
            case TvInputInfo.TYPE_COMPOSITE: return "Composite";
            case TvInputInfo.TYPE_COMPONENT: return "Component";
            case TvInputInfo.TYPE_SVIDEO: return "S-Video";
            case TvInputInfo.TYPE_SCART: return "SCART";
            case TvInputInfo.TYPE_VGA: return "VGA";
            case TvInputInfo.TYPE_DVI: return "DVI";
            case TvInputInfo.TYPE_DISPLAY_PORT: return "DisplayPort";
            case TvInputInfo.TYPE_TUNER: return "Tuner";
            default: return "Other";
        }
    }

    static CharSequence inputLabel(Context c, String inputId) {
        TvInputManager tv = (TvInputManager) c.getSystemService(Context.TV_INPUT_SERVICE);
        if (tv == null) return inputId;
        TvInputInfo in = tv.getTvInputInfo(inputId);
        if (in == null) return inputId;
        CharSequence label = in.loadCustomLabel(c);
        return TextUtils.isEmpty(label) ? in.loadLabel(c) : label;
    }

    static byte[] art(Context c, String pkg) {
        App a = apps(c).get(pkg);
        if (a == null) return null;
        PackageManager pm = c.getPackageManager();
        Drawable d = a.info.loadBanner(pm);
        if (d == null) d = a.info.loadIcon(pm);
        if (d == null) return null;
        return png(d);
    }

    private static byte[] png(Drawable d) {
        Bitmap bitmap;
        if (d instanceof BitmapDrawable && ((BitmapDrawable) d).getBitmap() != null) {
            bitmap = ((BitmapDrawable) d).getBitmap();
        } else {
            int w = d.getIntrinsicWidth() > 0 ? d.getIntrinsicWidth() : ART_MAX;
            int h = d.getIntrinsicHeight() > 0 ? d.getIntrinsicHeight() : ART_MAX;
            float scale = Math.min(1f, (float) ART_MAX / Math.max(w, h));
            w = Math.max(1, Math.round(w * scale));
            h = Math.max(1, Math.round(h * scale));
            bitmap = Bitmap.createBitmap(w, h, Bitmap.Config.ARGB_8888);
            Canvas canvas = new Canvas(bitmap);
            d.setBounds(0, 0, w, h);
            d.draw(canvas);
        }
        ByteArrayOutputStream out = new ByteArrayOutputStream();
        bitmap.compress(Bitmap.CompressFormat.PNG, 100, out);
        return out.toByteArray();
    }
}
