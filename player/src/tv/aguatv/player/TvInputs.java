package tv.aguatv.player;

import android.app.Activity;
import android.content.Context;
import android.content.Intent;
import android.media.tv.TvInputInfo;
import android.media.tv.TvInputManager;
import android.text.TextUtils;

import java.text.Collator;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;

final class TvInputs implements Ops.Inputs {
    private final Activity activity;

    TvInputs(Activity activity) {
        this.activity = activity;
    }

    static List<Ops.Input> passthrough(Context c) {
        TvInputManager tv = (TvInputManager) c.getSystemService(Context.TV_INPUT_SERVICE);
        List<Ops.Input> hdmi = new ArrayList<>(), other = new ArrayList<>();
        if (tv == null) return hdmi;
        for (TvInputInfo in : tv.getTvInputList()) {
            if (!in.isPassthroughInput() || in.isHidden(c)) continue;
            CharSequence label = in.loadCustomLabel(c);
            if (TextUtils.isEmpty(label)) label = in.loadLabel(c);
            boolean isHdmi = in.getType() == TvInputInfo.TYPE_HDMI;
            Ops.Input o = new Ops.Input(in.getId(), label == null ? in.getId() : label.toString(), isHdmi,
                tv.getInputState(in.getId()) != TvInputManager.INPUT_STATE_DISCONNECTED);
            (isHdmi ? hdmi : other).add(o);
        }
        Collator collator = Collator.getInstance();
        Collections.sort(hdmi, (a, b) -> collator.compare(a.label, b.label));
        Collections.sort(other, (a, b) -> collator.compare(a.label, b.label));
        hdmi.addAll(other);
        return hdmi;
    }

    static Ops.Input find(Context c, String id) {
        if (id == null) return null;
        for (Ops.Input in : passthrough(c)) if (in.id.equals(id)) return in;
        return null;
    }

    @Override
    public List<Ops.Input> list() {
        return passthrough(activity);
    }

    @Override
    public void open(String id) {
        Intent i = new Intent(activity, Input.class).putExtra(Input.EXTRA_ID, id);
        activity.runOnUiThread(() -> activity.startActivity(i));
    }
}
