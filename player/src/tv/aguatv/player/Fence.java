package tv.aguatv.player;

import java.io.UnsupportedEncodingException;
import java.net.URI;
import java.net.URISyntaxException;
import java.net.URLDecoder;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.Locale;

final class Fence {
    final String app;
    final String appOrigin;
    final String scope;
    final String authOrigin;
    final List<String> also;

    private Fence(String app, String appOrigin, String scope, String authOrigin, List<String> also) {
        this.app = app;
        this.appOrigin = appOrigin;
        this.scope = scope;
        this.authOrigin = authOrigin;
        this.also = also;
    }

    static Fence of(String app, String auth, String also) {
        URI a = parse(app);
        URI s = parse(auth);
        if (a == null || s == null) return null;
        String first = firstSegment(a.getRawPath());
        if (first == null) return null;
        List<String> extra = new ArrayList<>();
        if (also != null) {
            for (String one : also.split(",")) {
                URI o = parse(one.trim());
                if (o != null) extra.add(origin(o));
            }
        }
        return new Fence(app.trim(), origin(a), "/" + first + "/", origin(s), Collections.unmodifiableList(extra));
    }

    static String[] firstUse(String[] stored, String[] given) {
        if (stored != null && stored[0] != null) return of(stored[0], stored[1], stored[2]) == null ? null : stored;
        if (given == null || of(given[0], given[1], given[2]) == null) return null;
        return given;
    }

    boolean navigates(String url) {
        URI u = parse(url);
        if (u == null) return false;
        String o = origin(u);
        String p = path(u);
        if (o.equals(appOrigin)) return p.startsWith(scope) && clean(p);
        if (also.contains(o)) return p.equals("/");
        if (o.equals(authOrigin)) {
            if (p.equals("/logout")) return true;
            if (!(p.equals("/") || p.startsWith("/2fa/"))) return false;
            String rd = query(u, "rd");
            if (rd == null) return false;
            URI r = parse(rd);
            return r != null && origin(r).equals(appOrigin) && path(r).startsWith(scope) && clean(path(r));
        }
        return false;
    }

    boolean opens(String url) {
        URI u = parse(url);
        return u != null && !origin(u).equals(authOrigin) && navigates(url);
    }

    boolean ownsApp(String url) {
        URI u = parse(url);
        return u != null && origin(u).equals(appOrigin);
    }

    boolean authority(String url) {
        URI u = parse(url);
        return u != null && origin(u).equals(authOrigin);
    }

    boolean fetches(String url) {
        URI u = parse(url);
        if (u == null) return false;
        String o = origin(u);
        if (o.equals(appOrigin) || also.contains(o)) return true;
        if (!o.equals(authOrigin)) return false;
        String p = path(u).toLowerCase(Locale.ROOT);
        return !(p.startsWith("/settings") || p.contains("/register") || p.contains("/credential")
            || p.contains("/elevation") || p.contains("reset-password") || p.contains("/user/info/"));
    }

    private static URI parse(String s) {
        if (s == null) return null;
        try {
            URI u = new URI(s.trim());
            if (!"https".equalsIgnoreCase(u.getScheme()) || u.getHost() == null || u.getRawUserInfo() != null) return null;
            return u;
        } catch (URISyntaxException e) {
            return null;
        }
    }

    private static String origin(URI u) {
        return "https://" + u.getHost().toLowerCase(Locale.ROOT) + (u.getPort() == -1 ? "" : ":" + u.getPort());
    }

    private static String path(URI u) {
        String p = u.getRawPath();
        return p == null || p.isEmpty() ? "/" : p;
    }

    private static boolean clean(String rawPath) {
        String p = rawPath.toLowerCase(Locale.ROOT);
        if (p.contains("%2e") || p.contains("%2f") || p.contains("%5c") || p.contains("\\")) return false;
        for (String seg : p.split("/", -1)) {
            if (seg.equals(".") || seg.equals("..")) return false;
        }
        return true;
    }

    private static String firstSegment(String rawPath) {
        if (rawPath == null) return null;
        for (String seg : rawPath.split("/")) {
            if (!seg.isEmpty()) return clean("/" + seg) ? seg : null;
        }
        return null;
    }

    private static String query(URI u, String name) {
        String q = u.getRawQuery();
        if (q == null) return null;
        for (String pair : q.split("&")) {
            int eq = pair.indexOf('=');
            if (eq > 0 && pair.substring(0, eq).equals(name)) {
                try {
                    return URLDecoder.decode(pair.substring(eq + 1), "UTF-8");
                } catch (UnsupportedEncodingException | IllegalArgumentException e) {
                    return null;
                }
            }
        }
        return null;
    }
}
