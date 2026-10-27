package tv.aguatv.player;

import java.util.List;
import java.util.Map;

final class Ops {
    interface Clock {
        long millis();
    }

    static final class Request {
        final double id;
        final String path;
        final String body;
        final String input;

        Request(double id, String path, String body, String input) {
            this.id = id;
            this.path = path;
            this.body = body;
            this.input = input;
        }

        boolean local() {
            return path == null;
        }
    }

    static final class Input {
        final String id;
        final String label;
        final boolean hdmi;
        final boolean connected;

        Input(String id, String label, boolean hdmi, boolean connected) {
            this.id = id;
            this.label = label;
            this.hdmi = hdmi;
            this.connected = connected;
        }
    }

    interface Inputs {
        List<Input> list();

        void open(String id);
    }

    static final String REGISTER = "/_media/api/me";
    static final String STATE = "/_media/api/me/state";
    private static final int MAX_INPUT_ID = 256;
    private static final double BURST = 4, PER_SECOND = 2;

    private final Clock clock;
    private double tokens = BURST;
    private long last;

    Ops(Clock clock) {
        this.clock = clock;
        this.last = clock.millis();
    }

    synchronized Request accept(String message) {
        Object parsed = Json.parse(message);
        if (!(parsed instanceof Map)) return null;
        Map<?, ?> m = (Map<?, ?>) parsed;
        Object id = m.get("id"), op = m.get("op"), body = m.get("body");
        if (!(id instanceof Double) || !(body instanceof Map) || m.size() != 3) return null;
        double n = (Double) id;
        if (n != Math.rint(n) || n < 0 || n > 1e9) return null;
        Map<?, ?> b = (Map<?, ?>) body;
        Request r;
        if ("register".equals(op)) {
            String out = register(b);
            r = out == null ? null : new Request(n, REGISTER, out, null);
        } else if ("state".equals(op)) {
            String out = state(b);
            r = out == null ? null : new Request(n, STATE, out, null);
        } else if ("inputs".equals(op)) {
            r = b.isEmpty() ? new Request(n, null, null, null) : null;
        } else if ("input".equals(op)) {
            Object id1 = b.get("input");
            boolean ok = b.size() == 1 && id1 instanceof String && !((String) id1).isEmpty()
                && ((String) id1).length() <= MAX_INPUT_ID;
            r = ok ? new Request(n, null, null, (String) id1) : null;
        } else {
            return null;
        }
        if (r == null || !spend()) return null;
        return r;
    }

    static String answer(Request r, Inputs tv) {
        String id = Long.toString((long) r.id);
        List<Input> listed = tv.list();
        if (r.input == null) {
            StringBuilder out = new StringBuilder("[");
            for (Input in : listed) {
                if (out.length() > 1) out.append(',');
                out.append("{\"id\":").append(Json.quote(in.id))
                    .append(",\"label\":").append(Json.quote(in.label))
                    .append(",\"hdmi\":").append(in.hdmi)
                    .append(",\"connected\":").append(in.connected).append('}');
            }
            out.append(']');
            return "{\"id\":" + id + ",\"status\":200,\"body\":" + Json.quote(out.toString()) + "}";
        }
        for (Input in : listed) {
            if (in.id.equals(r.input)) {
                tv.open(in.id);
                return "{\"id\":" + id + ",\"status\":204}";
            }
        }
        return "{\"id\":" + id + ",\"status\":404}";
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
