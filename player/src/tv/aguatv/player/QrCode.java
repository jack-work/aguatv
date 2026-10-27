package tv.aguatv.player;

import java.nio.charset.StandardCharsets;

final class QrCode {
    private static final int[] ECC_PER_BLOCK = { -1, 10, 16, 26, 18, 24, 16, 18, 22, 22, 26 };
    private static final int[] BLOCKS = { -1, 1, 1, 1, 2, 2, 4, 4, 4, 5, 5 };
    static final int MAX_VERSION = 10;

    final int version;
    final int size;
    final int mask;
    private final boolean[][] modules;
    private final boolean[][] function;

    static QrCode encode(String text) {
        byte[] data = text.getBytes(StandardCharsets.UTF_8);
        for (int v = 1; v <= MAX_VERSION; v++) {
            int ccBits = v <= 9 ? 8 : 16;
            int capacity = dataCodewords(v) * 8;
            if (4 + ccBits + data.length * 8 <= capacity && data.length < (1 << ccBits)) {
                return new QrCode(v, data, ccBits);
            }
        }
        throw new IllegalArgumentException("too long for a version " + MAX_VERSION + " code");
    }

    boolean dark(int x, int y) {
        return x >= 0 && x < size && y >= 0 && y < size && modules[y][x];
    }

    private QrCode(int version, byte[] data, int ccBits) {
        this.version = version;
        this.size = version * 4 + 17;
        this.modules = new boolean[size][size];
        this.function = new boolean[size][size];
        drawFunctionPatterns();
        byte[] all = addEccAndInterleave(dataCodewords(data, ccBits));
        drawCodewords(all);
        int best = 0;
        int bestScore = Integer.MAX_VALUE;
        for (int m = 0; m < 8; m++) {
            applyMask(m);
            drawFormatBits(m);
            int score = penalty();
            if (score < bestScore) {
                best = m;
                bestScore = score;
            }
            applyMask(m);
        }
        this.mask = best;
        applyMask(best);
        drawFormatBits(best);
    }

    private static int rawDataModules(int v) {
        int result = (16 * v + 128) * v + 64;
        if (v >= 2) {
            int align = v / 7 + 2;
            result -= (25 * align - 10) * align - 55;
            if (v >= 7) result -= 36;
        }
        return result;
    }

    private static int dataCodewords(int v) {
        return rawDataModules(v) / 8 - ECC_PER_BLOCK[v] * BLOCKS[v];
    }

    private byte[] dataCodewords(byte[] data, int ccBits) {
        int capacity = dataCodewords(version);
        Bits bits = new Bits(capacity * 8);
        bits.append(0x4, 4);
        bits.append(data.length, ccBits);
        for (byte b : data) bits.append(b & 0xFF, 8);
        bits.append(0, Math.min(4, capacity * 8 - bits.length));
        bits.append(0, (8 - bits.length % 8) % 8);
        for (int pad = 0xEC; bits.length < capacity * 8; pad ^= 0xEC ^ 0x11) bits.append(pad, 8);
        return bits.bytes();
    }

    private static final class Bits {
        final byte[] out;
        int length;

        Bits(int capacityBits) {
            out = new byte[capacityBits / 8];
        }

        void append(int value, int count) {
            for (int i = count - 1; i >= 0; i--, length++) {
                if (((value >>> i) & 1) != 0) out[length >>> 3] |= (byte) (0x80 >>> (length & 7));
            }
        }

        byte[] bytes() {
            return out;
        }
    }

    private byte[] addEccAndInterleave(byte[] data) {
        int blocks = BLOCKS[version];
        int eccLen = ECC_PER_BLOCK[version];
        int raw = rawDataModules(version) / 8;
        int shortBlocks = blocks - raw % blocks;
        int shortLen = raw / blocks;
        byte[] divisor = rsDivisor(eccLen);
        byte[][] all = new byte[blocks][];
        for (int i = 0, k = 0; i < blocks; i++) {
            int datLen = shortLen - eccLen + (i < shortBlocks ? 0 : 1);
            byte[] dat = new byte[datLen];
            System.arraycopy(data, k, dat, 0, datLen);
            k += datLen;
            byte[] ecc = rsRemainder(dat, divisor);
            byte[] block = new byte[shortLen + 1];
            System.arraycopy(dat, 0, block, 0, datLen);
            System.arraycopy(ecc, 0, block, block.length - eccLen, eccLen);
            all[i] = block;
        }
        byte[] result = new byte[raw];
        int n = 0;
        for (int i = 0; i < all[0].length; i++) {
            for (int j = 0; j < blocks; j++) {
                if (i != shortLen - eccLen || j >= shortBlocks) result[n++] = all[j][i];
            }
        }
        return result;
    }

    private static byte[] rsDivisor(int degree) {
        byte[] result = new byte[degree];
        result[degree - 1] = 1;
        int root = 1;
        for (int i = 0; i < degree; i++) {
            for (int j = 0; j < degree; j++) {
                result[j] = (byte) gfMul(result[j] & 0xFF, root);
                if (j + 1 < degree) result[j] ^= result[j + 1];
            }
            root = gfMul(root, 0x02);
        }
        return result;
    }

    private static byte[] rsRemainder(byte[] data, byte[] divisor) {
        byte[] result = new byte[divisor.length];
        for (byte b : data) {
            int factor = (b ^ result[0]) & 0xFF;
            System.arraycopy(result, 1, result, 0, result.length - 1);
            result[result.length - 1] = 0;
            for (int i = 0; i < result.length; i++) result[i] ^= (byte) gfMul(divisor[i] & 0xFF, factor);
        }
        return result;
    }

    private static int gfMul(int x, int y) {
        int z = 0;
        for (int i = 7; i >= 0; i--) {
            z = (z << 1) ^ ((z >>> 7) * 0x11D);
            z ^= ((y >>> i) & 1) * x;
        }
        return z;
    }

    private void set(int x, int y, boolean dark) {
        modules[y][x] = dark;
        function[y][x] = true;
    }

    private void drawFunctionPatterns() {
        for (int i = 0; i < size; i++) {
            set(6, i, i % 2 == 0);
            set(i, 6, i % 2 == 0);
        }
        finder(3, 3);
        finder(size - 4, 3);
        finder(3, size - 4);
        int[] align = alignmentPositions();
        int n = align.length;
        for (int i = 0; i < n; i++) {
            for (int j = 0; j < n; j++) {
                if (!(i == 0 && j == 0 || i == 0 && j == n - 1 || i == n - 1 && j == 0)) alignment(align[i], align[j]);
            }
        }
        drawFormatBits(0);
        drawVersion();
    }

    private int[] alignmentPositions() {
        if (version == 1) return new int[0];
        int n = version / 7 + 2;
        int step = (version * 4 + n * 2 + 1) / (n * 2 - 2) * 2;
        int[] result = new int[n];
        result[0] = 6;
        for (int i = n - 1, pos = size - 7; i >= 1; i--, pos -= step) result[i] = pos;
        return result;
    }

    private void finder(int x, int y) {
        for (int dy = -4; dy <= 4; dy++) {
            for (int dx = -4; dx <= 4; dx++) {
                int dist = Math.max(Math.abs(dx), Math.abs(dy));
                int xx = x + dx, yy = y + dy;
                if (xx >= 0 && xx < size && yy >= 0 && yy < size) set(xx, yy, dist != 2 && dist != 4);
            }
        }
    }

    private void alignment(int x, int y) {
        for (int dy = -2; dy <= 2; dy++) {
            for (int dx = -2; dx <= 2; dx++) set(x + dx, y + dy, Math.max(Math.abs(dx), Math.abs(dy)) != 1);
        }
    }

    private static boolean bit(int x, int i) {
        return ((x >>> i) & 1) != 0;
    }

    private void drawFormatBits(int mask) {
        int data = mask;
        int rem = data;
        for (int i = 0; i < 10; i++) rem = (rem << 1) ^ ((rem >>> 9) * 0x537);
        int bits = ((data << 10) | rem) ^ 0x5412;
        for (int i = 0; i <= 5; i++) set(8, i, bit(bits, i));
        set(8, 7, bit(bits, 6));
        set(8, 8, bit(bits, 7));
        set(7, 8, bit(bits, 8));
        for (int i = 9; i < 15; i++) set(14 - i, 8, bit(bits, i));
        for (int i = 0; i < 8; i++) set(size - 1 - i, 8, bit(bits, i));
        for (int i = 8; i < 15; i++) set(8, size - 15 + i, bit(bits, i));
        set(8, size - 8, true);
    }

    private void drawVersion() {
        if (version < 7) return;
        int rem = version;
        for (int i = 0; i < 12; i++) rem = (rem << 1) ^ ((rem >>> 11) * 0x1F25);
        int bits = version << 12 | rem;
        for (int i = 0; i < 18; i++) {
            boolean b = bit(bits, i);
            int a = size - 11 + i % 3, c = i / 3;
            set(a, c, b);
            set(c, a, b);
        }
    }

    private void drawCodewords(byte[] data) {
        int i = 0;
        for (int right = size - 1; right >= 1; right -= 2) {
            if (right == 6) right = 5;
            for (int vert = 0; vert < size; vert++) {
                for (int j = 0; j < 2; j++) {
                    int x = right - j;
                    boolean upward = ((right + 1) & 2) == 0;
                    int y = upward ? size - 1 - vert : vert;
                    if (!function[y][x] && i < data.length * 8) {
                        modules[y][x] = bit(data[i >>> 3] & 0xFF, 7 - (i & 7));
                        i++;
                    }
                }
            }
        }
    }

    private void applyMask(int m) {
        for (int y = 0; y < size; y++) {
            for (int x = 0; x < size; x++) {
                boolean invert;
                switch (m) {
                    case 0: invert = (x + y) % 2 == 0; break;
                    case 1: invert = y % 2 == 0; break;
                    case 2: invert = x % 3 == 0; break;
                    case 3: invert = (x + y) % 3 == 0; break;
                    case 4: invert = (x / 3 + y / 2) % 2 == 0; break;
                    case 5: invert = x * y % 2 + x * y % 3 == 0; break;
                    case 6: invert = (x * y % 2 + x * y % 3) % 2 == 0; break;
                    default: invert = ((x + y) % 2 + x * y % 3) % 2 == 0; break;
                }
                if (invert && !function[y][x]) modules[y][x] = !modules[y][x];
            }
        }
    }

    private int penalty() {
        int result = 0;
        for (int pass = 0; pass < 2; pass++) {
            for (int a = 0; a < size; a++) {
                boolean runColor = false;
                int run = 0;
                int[] history = new int[7];
                for (int b = 0; b < size; b++) {
                    boolean c = pass == 0 ? modules[a][b] : modules[b][a];
                    if (c == runColor) {
                        run++;
                        if (run == 5) result += 3;
                        else if (run > 5) result++;
                    } else {
                        addHistory(run, history);
                        if (!runColor) result += finderLike(history) * 40;
                        runColor = c;
                        run = 1;
                    }
                }
                if (runColor) {
                    addHistory(run, history);
                    run = 0;
                }
                addHistory(run + size, history);
                result += finderLike(history) * 40;
            }
        }
        for (int y = 0; y < size - 1; y++) {
            for (int x = 0; x < size - 1; x++) {
                boolean c = modules[y][x];
                if (c == modules[y][x + 1] && c == modules[y + 1][x] && c == modules[y + 1][x + 1]) result += 3;
            }
        }
        int dark = 0;
        for (boolean[] row : modules) for (boolean c : row) if (c) dark++;
        int total = size * size;
        int k = (Math.abs(dark * 20 - total * 10) + total - 1) / total - 1;
        return result + k * 10;
    }

    private void addHistory(int run, int[] history) {
        if (history[0] == 0) run += size;
        System.arraycopy(history, 0, history, 1, history.length - 1);
        history[0] = run;
    }

    private static int finderLike(int[] h) {
        int n = h[1];
        boolean core = n > 0 && h[2] == n && h[3] == n * 3 && h[4] == n && h[5] == n;
        return (core && h[0] >= n * 4 && h[6] >= n ? 1 : 0) + (core && h[6] >= n * 4 && h[0] >= n ? 1 : 0);
    }
}
