package org.bluray.ti;

import witty.bdj.Host;

/** Reads the disc's own description files, through the same door disc code has. */
final class DiscFiles {
    private DiscFiles() {}
    /** The first n bytes of a file under the disc's root, or null when it cannot be read. */
    static byte[] head(String name, int n) {
        try {
            java.io.InputStream in = new witty.shim.FileInputStream(Host.vfsRoot + java.io.File.separator + name.replace('/', java.io.File.separatorChar));
            try {
                byte[] b = new byte[n];
                int got = 0;
                for (int r; got < n && (r = in.read(b, got, n - got)) > 0; ) got += r;
                return java.util.Arrays.copyOf(b, got);
            } finally {
                in.close();
            }
        } catch (java.io.IOException e) {
            return null;
        }
    }
    static int u32(byte[] b, int o) { return (b[o] & 255) << 24 | (b[o + 1] & 255) << 16 | (b[o + 2] & 255) << 8 | (b[o + 3] & 255); }
    static int u16(byte[] b, int o) { return (b[o] & 255) << 8 | (b[o + 1] & 255); }
}
