package tv.aguatv.player;

import java.util.Map;

final class Ops {
    interface Clock {
        long millis();
    }

    static final class Write {
        final double id;
        final String path;
        final String body;

        Write(double id, String path, String body) {
            this.id = id;
            this.path = path;
            this.body = body;
        }
    }

    static final String REGISTER = "/_media/api/me";
    static final String STATE = "/_media/api/me/state";
    private static final double BURST = 4, PER_SECOND = 2;

    private final Clock clock;
    private double tokens = BURST;
    private long last;

    Ops(Clock clock) {
        this.clock = clock;
        this.last = clock.millis();
    }

    synchronized Write accept(String message) {
        Object parsed = Json.parse(message);
        if (!(parsed instanceof Map)) return null;
        Map<?, ?> m = (Map<?, ?>) parsed;
        Object id = m.get("id"), op = m.get("op"), body = m.get("body");
        if (!(id instanceof Double) || !(body instanceof Map) || m.size() != 3) return null;
        double n = (Double) id;
        if (n != Math.rint(n) || n < 0 || n > 1e9) return null;
        String out;
        String path;
        if ("register".equals(op)) {
            out = register((Map<?, ?>) body);
            path = REGISTER;
        } else if ("state".equals(op)) {
            out = state((Map<?, ?>) body);
            path = STATE;
        } else {
            return null;
        }
        if (out == null || !spend()) return null;
        return new Write(n, path, out);
    }

    private boolean spend() {
        long now = clock.millis();
        tokens = Math.min(BURST, tokens + Math.max(0, now - last) * PER_SECOND / 1000.0);
        last = now;
        if (tokens < 1) return false;
        tokens -= 1;
        return true;
    }

    static String register(Map<?, ?> body) {
        Object name = body.get("name");
        if (!(name instanceof String)) return null;
        String n = ((String) name).trim();
        int points = n.codePointCount(0, n.length());
        if (points < 1 || points > 64 || !printable(n)) return null;
        return "{\"name\":" + Json.quote(n) + ",\"remote\":false}";
    }

    static String state(Map<?, ?> body) {
        Object title = body.get("title"), key = body.get("key"), position = body.get("position"),
            duration = body.get("duration"), playing = body.get("playing");
        if (!(title instanceof String) || ((String) title).length() > 256) return null;
        if (!(key instanceof String) || ((String) key).length() > 1024) return null;
        if (!finite(position) || !finite(duration) || !(playing instanceof Boolean)) return null;
        return "{\"title\":" + Json.quote((String) title) + ",\"key\":" + Json.quote((String) key)
            + ",\"position\":" + position + ",\"duration\":" + duration + ",\"playing\":" + playing + "}";
    }

    private static boolean finite(Object v) {
        return v instanceof Double && !((Double) v).isNaN() && !((Double) v).isInfinite() && (Double) v >= 0;
    }

    private static boolean printable(String s) {
        for (int k = 0; k < s.length(); ) {
            int c = s.codePointAt(k);
            int t = Character.getType(c);
            if (t == Character.CONTROL || t == Character.FORMAT || t == Character.SURROGATE
                || t == Character.PRIVATE_USE || t == Character.UNASSIGNED
                || t == Character.LINE_SEPARATOR || t == Character.PARAGRAPH_SEPARATOR) return false;
            k += Character.charCount(c);
        }
        return true;
    }
}
