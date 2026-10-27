package tv.aguatv.player;

import java.util.LinkedHashMap;
import java.util.Map;

final class Json {
    static final int MAX = 8192;

    private final String s;
    private int i;
    private int depth;

    private Json(String s) {
        this.s = s;
    }

    static Object parse(String s) {
        if (s == null || s.length() > MAX) return null;
        Json p = new Json(s);
        try {
            Object v = p.value();
            p.space();
            return p.i == s.length() ? v : null;
        } catch (RuntimeException e) {
            return null;
        }
    }

    private void space() {
        while (i < s.length() && " \t\r\n".indexOf(s.charAt(i)) >= 0) i++;
    }

    private Object value() {
        space();
        char c = s.charAt(i);
        if (c == '{') return object();
        if (c == '"') return string();
        if (s.startsWith("true", i)) { i += 4; return Boolean.TRUE; }
        if (s.startsWith("false", i)) { i += 5; return Boolean.FALSE; }
        if (s.startsWith("null", i)) { i += 4; return NULL; }
        if (c == '-' || c >= '0' && c <= '9') return number();
        throw new IllegalArgumentException("unexpected " + c);
    }

    static final Object NULL = new Object();

    private Map<String, Object> object() {
        if (++depth > 4) throw new IllegalArgumentException("too deep");
        Map<String, Object> out = new LinkedHashMap<>();
        i++;
        space();
        if (s.charAt(i) == '}') { i++; depth--; return out; }
        while (true) {
            space();
            String k = string();
            space();
            if (s.charAt(i++) != ':') throw new IllegalArgumentException("colon");
            if (out.put(k, value()) != null) throw new IllegalArgumentException("duplicate key");
            space();
            char c = s.charAt(i++);
            if (c == '}') break;
            if (c != ',') throw new IllegalArgumentException("comma");
        }
        depth--;
        return out;
    }

    private String string() {
        if (s.charAt(i) != '"') throw new IllegalArgumentException("quote");
        i++;
        StringBuilder b = new StringBuilder();
        while (true) {
            char c = s.charAt(i++);
            if (c == '"') return b.toString();
            if (c < 0x20) throw new IllegalArgumentException("control character");
            if (c != '\\') { b.append(c); continue; }
            char e = s.charAt(i++);
            switch (e) {
                case '"': b.append('"'); break;
                case '\\': b.append('\\'); break;
                case '/': b.append('/'); break;
                case 'b': b.append('\b'); break;
                case 'f': b.append('\f'); break;
                case 'n': b.append('\n'); break;
                case 'r': b.append('\r'); break;
                case 't': b.append('\t'); break;
                case 'u': b.append((char) Integer.parseInt(s.substring(i, i + 4), 16)); i += 4; break;
                default: throw new IllegalArgumentException("escape");
            }
        }
    }

    private Double number() {
        int start = i;
        if (s.charAt(i) == '-') i++;
        while (i < s.length() && "0123456789.eE+-".indexOf(s.charAt(i)) >= 0) i++;
        return Double.valueOf(s.substring(start, i));
    }

    static String quote(String v) {
        StringBuilder b = new StringBuilder("\"");
        for (int k = 0; k < v.length(); k++) {
            char c = v.charAt(k);
            if (c == '"' || c == '\\') b.append('\\').append(c);
            else if (c < 0x20 || c == 0x2028 || c == 0x2029) b.append(String.format("\\u%04x", (int) c));
            else b.append(c);
        }
        return b.append('"').toString();
    }
}
