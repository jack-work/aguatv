package tv.aguatv.player;

import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;

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

    private static final class Fake implements Ops.Inputs {
        final List<String> opened = new ArrayList<>();
        int listed;

        @Override
        public List<Ops.Input> list() {
            listed++;
            return Arrays.asList(new Ops.Input("hw/HDMI1", "HDMI 1", true, true),
                new Ops.Input("hw/AV", "Composite", false, false));
        }

        @Override
        public void open(String id) {
            opened.add(id);
        }
    }

    private static void inputs() {
        Ops.Request r = fresh().accept("{\"id\":7,\"op\":\"inputs\",\"body\":{}}");
        check("inputs with an empty body is local", r != null && r.local() && r.input == null && r.id == 7);
        Fake tv = new Fake();
        String reply = Ops.answer(r, tv);
        check("inputs lists every passthrough input, opening none",
            reply.equals("{\"id\":7,\"status\":200,\"body\":\"[{\\\"id\\\":\\\"hw/HDMI1\\\",\\\"label\\\":\\\"HDMI 1\\\",\\\"hdmi\\\":true,\\\"connected\\\":true},"
                + "{\\\"id\\\":\\\"hw/AV\\\",\\\"label\\\":\\\"Composite\\\",\\\"hdmi\\\":false,\\\"connected\\\":false}]\"}")
            && tv.opened.isEmpty());
        check("inputs with any body field is dropped", fresh().accept("{\"id\":7,\"op\":\"inputs\",\"body\":{\"x\":1}}") == null);
        check("inputs without a body object is dropped", fresh().accept("{\"id\":7,\"op\":\"inputs\",\"body\":[]}") == null);

        r = fresh().accept("{\"id\":8,\"op\":\"input\",\"body\":{\"input\":\"hw/HDMI1\"}}");
        check("input names one input, locally", r != null && r.local() && "hw/HDMI1".equals(r.input));
        tv = new Fake();
        check("a listed input opens and answers 204",
            Ops.answer(r, tv).equals("{\"id\":8,\"status\":204}") && tv.opened.equals(Arrays.asList("hw/HDMI1")));
        tv = new Fake();
        r = fresh().accept("{\"id\":9,\"op\":\"input\",\"body\":{\"input\":\"hw/HDMI9\"}}");
        check("an unlisted input answers 404 and opens nothing",
            Ops.answer(r, tv).equals("{\"id\":9,\"status\":404}") && tv.opened.isEmpty());
        tv = new Fake();
        r = fresh().accept("{\"id\":9,\"op\":\"input\",\"body\":{\"input\":\"hw/hdmi1\"}}");
        check("the match is exact", Ops.answer(r, tv).endsWith("404}") && tv.opened.isEmpty());
        r = fresh().accept("{\"id\":9,\"op\":\"input\",\"body\":{\"input\":\"hw/HDMI1 \"}}");
        check("a padded id is not trimmed into a match", Ops.answer(r, new Fake()).endsWith("404}"));
        check("an input of the wrong type is dropped", fresh().accept("{\"id\":9,\"op\":\"input\",\"body\":{\"input\":1}}") == null);
        check("an empty input is dropped", fresh().accept("{\"id\":9,\"op\":\"input\",\"body\":{\"input\":\"\"}}") == null);
        check("an input with extra keys is dropped",
            fresh().accept("{\"id\":9,\"op\":\"input\",\"body\":{\"input\":\"hw/HDMI1\",\"app\":\"x\"}}") == null);
        check("an input missing its key is dropped", fresh().accept("{\"id\":9,\"op\":\"input\",\"body\":{}}") == null);
        String id256 = new String(new char[256]).replace('\0', 'i');
        check("an id of 256 characters is accepted for lookup",
            fresh().accept("{\"id\":9,\"op\":\"input\",\"body\":{\"input\":\"" + id256 + "\"}}") != null);
        check("an id of 257 characters is dropped",
            fresh().accept("{\"id\":9,\"op\":\"input\",\"body\":{\"input\":\"" + id256 + "i\"}}") == null);
        check("register and state never touch inputs", !fresh().accept("{\"id\":1,\"op\":\"register\",\"body\":{\"name\":\"TV\"}}").local());
        Ops ops = fresh();
        int passed = 0;
        for (int k = 0; k < 10; k++) if (ops.accept("{\"id\":8,\"op\":\"input\",\"body\":{\"input\":\"hw/HDMI1\"}}") != null) passed++;
        check("input shares the bucket", passed == 4);
        check("and so does inputs, after it", ops.accept("{\"id\":7,\"op\":\"inputs\",\"body\":{}}") == null);
    }

    public static void main(String[] args) {
        inputs();
        Ops ops = fresh();
        Ops.Request w = ops.accept("{\"id\":1,\"op\":\"register\",\"body\":{\"name\":\"Living room\",\"remote\":true,\"kind\":\"admin\"}}");
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
