package tv.aguatv.player;

public final class FenceTest {
    private static int failures;

    private static void check(String name, boolean ok) {
        System.out.println((ok ? "ok   " : "FAIL ") + name);
        if (!ok) failures++;
    }

    public static void main(String[] args) {
        Fence f = Fence.of("https://files.example.org/_media/app/home", "https://auth.example.org", "https://tv.example.org");
        String rd = "https%3A%2F%2Ffiles.example.org%2F_media%2Fapp%2Fhome";
        check("the app loads", f.navigates("https://files.example.org/_media/app/home?w=a%2Fb.mp4"));
        check("the app's own files outside its scope do not open as pages", !f.navigates("https://files.example.org/dad/notes.pdf"));
        check("a dot segment cannot climb out of the scope", !f.navigates("https://files.example.org/_media/../dad/x"));
        check("an encoded dot segment cannot either", !f.navigates("https://files.example.org/_media/%2e%2e/dad/x"));
        check("the TV name opens only at its root", f.navigates("https://tv.example.org/") && !f.navigates("https://tv.example.org/x"));
        check("the sign-in page opens with a way back to the app", f.navigates("https://auth.example.org/?rd=" + rd + "&rm=GET"));
        check("the second factor opens with a way back to the app", f.navigates("https://auth.example.org/2fa/one-time-password?rd=" + rd));
        check("the sign-in page without a way back does not", !f.navigates("https://auth.example.org/"));
        check("a way back somewhere else does not", !f.navigates("https://auth.example.org/?rd=https%3A%2F%2Fkeel.example.org%2F"));
        check("a way back that climbs out does not", !f.navigates("https://auth.example.org/?rd=https%3A%2F%2Ffiles.example.org%2F_media%2F..%2Fx"));
        check("the portal's settings never open", !f.navigates("https://auth.example.org/settings/two-factor-authentication?rd=" + rd));
        check("signing out opens", f.navigates("https://auth.example.org/logout"));
        check("another site never opens", !f.navigates("https://keel.example.org/") && !f.navigates("https://example.com/"));
        check("plain http never opens", !f.navigates("http://files.example.org/_media/app/home"));
        check("credentials in a URL never open", !f.navigates("https://x@files.example.org/_media/app/home"));
        check("other schemes never open", !f.navigates("intent://x#Intent;end") && !f.navigates("javascript:alert(1)") && !f.navigates("file:///sdcard/x"));
        check("the app's assets and media load", f.fetches("https://files.example.org/_media/r/a/b.mp4/0123456789ab/v720_00001.m4s") && f.fetches("https://files.example.org/a/b.mp4"));
        check("the portal's sign-in calls load", f.fetches("https://auth.example.org/api/firstfactor") && f.fetches("https://auth.example.org/api/secondfactor/totp") && f.fetches("https://auth.example.org/api/user/info"));
        check("registering a second factor is refused", !f.fetches("https://auth.example.org/api/secondfactor/totp/register") && !f.fetches("https://auth.example.org/api/secondfactor/webauthn/credentials"));
        check("elevation and settings are refused", !f.fetches("https://auth.example.org/api/user/session/elevation") && !f.fetches("https://auth.example.org/settings"));
        check("changing the second factor method is refused", !f.fetches("https://auth.example.org/api/user/info/2fa_method"));
        check("no other host loads anything", !f.fetches("https://keel.example.org/x.js") && !f.fetches("https://cdn.example.com/x.js") && !f.fetches("http://files.example.org/x"));
        check("the web view opens the app and the redirect origin", f.opens("https://files.example.org/_media/app/home") && f.opens("https://tv.example.org/"));
        check("the web view never opens the portal, even its sign-in page", !f.opens("https://auth.example.org/?rd=" + rd) && !f.opens("https://auth.example.org/logout"));
        check("only the app origin gets the token", f.ownsApp("https://files.example.org/x") && !f.ownsApp("https://tv.example.org/") && !f.ownsApp("https://auth.example.org/") && !f.ownsApp("https://files.example.org.evil.net/x") && !f.ownsApp("http://files.example.org/x"));
        check("the sign-in endpoints must be on the portal", f.authority("https://auth.example.org/api/oidc/token") && !f.authority("https://evil.example.net/api/oidc/token"));
        check("an app without a path is refused", Fence.of("https://files.example.org/", "https://auth.example.org", null) == null);
        check("an http app is refused", Fence.of("http://files.example.org/_media/app/home", "https://auth.example.org", null) == null);
        String[] mine = { "https://files.example.org/_media/app/home", "https://auth.example.org", "https://tv.example.org" };
        String[] theirs = { "https://evil.example.net/_media/app/home", "https://evil.example.net", "" };
        check("the first configuration is taken", Fence.firstUse(new String[3], mine) == mine);
        check("a second configuration is ignored", Fence.firstUse(mine, theirs) == mine);
        check("an unusable first configuration is not stored", Fence.firstUse(new String[3], new String[] { "http://x/y", "https://a", null }) == null);
        check("with nothing stored and nothing given there is no app", Fence.firstUse(new String[3], new String[3]) == null);
        if (failures > 0) {
            System.out.println(failures + " FAILED");
            System.exit(1);
        }
    }
}
