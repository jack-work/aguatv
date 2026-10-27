package tv.aguatv.player;

import java.io.FileOutputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;

public final class QrCodeTest {
    private static int failures;

    private static void check(String name, boolean ok) {
        System.out.println((ok ? "ok   " : "FAIL ") + name);
        if (!ok) failures++;
    }

    private static String repeat(char c, int n) {
        StringBuilder b = new StringBuilder();
        for (int i = 0; i < n; i++) b.append(c);
        return b.toString();
    }

    private static void write(String dir, int n, String text) throws IOException {
        QrCode q = QrCode.encode(text);
        int scale = 4, quiet = 4, side = (q.size + quiet * 2) * scale;
        StringBuilder out = new StringBuilder("P1\n" + side + " " + side + "\n");
        for (int py = 0; py < side; py++) {
            for (int px = 0; px < side; px++) {
                out.append(q.dark(px / scale - quiet, py / scale - quiet) ? '1' : '0');
                out.append(px + 1 == side ? '\n' : ' ');
            }
        }
        try (FileOutputStream f = new FileOutputStream(dir + "/" + n + ".pbm")) {
            f.write(out.toString().getBytes(StandardCharsets.US_ASCII));
        }
        try (FileOutputStream f = new FileOutputStream(dir + "/" + n + ".txt")) {
            f.write(text.getBytes(StandardCharsets.UTF_8));
        }
        System.out.println("     wrote " + n + ": version " + q.version + ", mask " + q.mask + ", " + text.length() + " chars");
    }

    public static void main(String[] args) throws IOException {
        check("14 bytes fit version 1", QrCode.encode(repeat('a', 14)).version == 1);
        check("15 bytes need version 2", QrCode.encode(repeat('a', 15)).version == 2);
        check("213 bytes fit version 10", QrCode.encode(repeat('a', 213)).version == 10);
        boolean refused = false;
        try {
            QrCode.encode(repeat('a', 214));
        } catch (IllegalArgumentException e) {
            refused = true;
        }
        check("214 bytes are refused", refused);
        String[] samples = {
            "HELLO",
            "https://auth.example.org/devices?user_code=ABCD-EFGH",
            "https://auth.example.org/consent/openid/device-authorization?user_code=WXYZ-1234-QRST",
            repeat('x', 110),
            repeat('7', 90),
            repeat('q', 130),
            repeat('Z', 150),
            repeat('m', 180),
            repeat('k', 213),
        };
        for (int i = 0; i < samples.length; i++) write(args[0], i, samples[i]);
        if (failures > 0) {
            System.out.println(failures + " FAILED");
            System.exit(1);
        }
    }
}
