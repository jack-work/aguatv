package tv.aguatv.player;

public final class OpsTest {
    private static int failures;
    private static long now;

    private static void check(String name, boolean ok) {
        System.out.println((ok ? "ok   " : "FAIL ") + name);
        if (!ok) failures++;
    }

    private static Ops fresh() {
        now = 0;
        return new Ops(() -> now);
    }

    public static void main(String[] args) {
        Ops ops = fresh();
        Ops.Write w = ops.accept("{\"id\":1,\"op\":\"register\",\"body\":{\"name\":\"Living room\",\"remote\":true,\"kind\":\"admin\"}}");
        check("register posts to /api/me", w != null && w.path.equals(Ops.REGISTER) && w.id == 1);
        check("register never asks for remote and drops other fields", w != null && w.body.equals("{\"name\":\"Living room\",\"remote\":false}"));
        check("a name of 64 characters registers", fresh().accept("{\"id\":1,\"op\":\"register\",\"body\":{\"name\":\"" + new String(new char[64]).replace('\0', 'n') + "\"}}") != null);
        check("a name of 65 characters does not", fresh().accept("{\"id\":1,\"op\":\"register\",\"body\":{\"name\":\"" + new String(new char[65]).replace('\0', 'n') + "\"}}") == null);
        check("an empty name does not", fresh().accept("{\"id\":1,\"op\":\"register\",\"body\":{\"name\":\"  \"}}") == null);
        check("a name with a control character does not", fresh().accept("{\"id\":1,\"op\":\"register\",\"body\":{\"name\":\"a\\u0007b\"}}") == null);
        check("a name with a direction override does not", fresh().accept("{\"id\":1,\"op\":\"register\",\"body\":{\"name\":\"a\\u202eb\"}}") == null);
        String good = "{\"id\":2,\"op\":\"state\",\"body\":{\"title\":\"t\",\"key\":\"a/b.mp4\",\"position\":1.5,\"duration\":30,\"playing\":true,\"extra\":1}}";
        w = fresh().accept(good);
        check("state posts to /api/me/state with exactly its five fields",
            w != null && w.path.equals(Ops.STATE) && w.body.equals("{\"title\":\"t\",\"key\":\"a/b.mp4\",\"position\":1.5,\"duration\":30.0,\"playing\":true}"));
        check("a negative position is dropped", fresh().accept(good.replace("1.5", "-1")) == null);
        check("a huge exponent is dropped", fresh().accept(good.replace("1.5", "1e999")) == null);
        check("a string position is dropped", fresh().accept(good.replace("1.5", "\"1\"")) == null);
        check("a long title is dropped", fresh().accept(good.replace("\"t\"", "\"" + new String(new char[257]).replace('\0', 't') + "\"")) == null);
        check("a missing field is dropped", fresh().accept(good.replace(",\"playing\":true", "")) == null);
        check("any other op is dropped", fresh().accept("{\"id\":3,\"op\":\"cast\",\"body\":{\"key\":\"x\"}}") == null);
        check("a write to another path is impossible", fresh().accept("{\"id\":3,\"op\":\"/api/screens/x/cast\",\"body\":{}}") == null);
        check("extra top-level fields are dropped", fresh().accept("{\"id\":3,\"op\":\"state\",\"body\":{},\"path\":\"/x\"}") == null);
        check("garbage is dropped", fresh().accept("not json") == null && fresh().accept("[1]") == null && fresh().accept(null) == null);
        check("a duplicate key is dropped", fresh().accept("{\"id\":1,\"id\":2,\"op\":\"state\",\"body\":{}}") == null);
        ops = fresh();
        int passed = 0;
        for (int k = 0; k < 10; k++) if (ops.accept(good) != null) passed++;
        check("a burst is cut to four", passed == 4);
        now += 1000;
        passed = 0;
        for (int k = 0; k < 10; k++) if (ops.accept(good) != null) passed++;
        check("then two more a second", passed == 2);
        if (failures > 0) {
            System.out.println(failures + " FAILED");
            System.exit(1);
        }
    }
}
