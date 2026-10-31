package witty.shim;

import java.io.IOException;
import witty.bdj.Host;

/** Where a disc's code may read and write. Everything else does not exist for it. */
final class Roots {
    private Roots() {}

    /** Most a disc may store in its persistent area and temporary area. */
    static final long QUOTA = 64L << 20;
    private static long used = -1;

    static java.io.File resolve(String path, boolean write) throws IOException {
        if (path == null) throw new NullPointerException();
        if (!plain(path)) throw new java.io.FileNotFoundException(path + " is not available to disc applications");
        java.io.File f = new java.io.File(path);
        if (!f.isAbsolute()) f = new java.io.File(Host.vfsRoot, path);
        String c = f.getCanonicalPath();
        if (under(c, Host.persistRoot) || under(c, Host.tempRoot)) return f;
        if (!write && under(c, Host.vfsRoot)) return f;
        throw new java.io.FileNotFoundException(path + " is not available to disc applications");
    }

    /**
     * Windows gives some names a meaning of their own, and only asking about a share
     * name already reaches over the network: \\host\share, \\.\device and a file's
     * alternate streams never name anything a disc owns.
     */
    static boolean plain(String path) {
        if (java.io.File.separatorChar != '\\') return path.indexOf('\0') < 0;
        if (path.indexOf('\0') >= 0 || path.startsWith("\\\\") || path.startsWith("//") || path.startsWith("\\/") || path.startsWith("/\\")) return false;
        int colon = path.indexOf(':');
        return colon < 0 || (colon == 1 && path.indexOf(':', 2) < 0);
    }

    private static boolean under(String path, String root) {
        return path.equals(root) || path.startsWith(root + java.io.File.separator);
    }

    static boolean known(String path) {
        try {
            resolve(path, false);
            return true;
        } catch (IOException e) {
            return false;
        }
    }

    static synchronized void charge(long n) throws IOException {
        if (used < 0) used = size(new java.io.File(Host.persistRoot)) + size(new java.io.File(Host.tempRoot));
        if (used + n > QUOTA) throw new IOException("the disc's storage is full");
        used += n;
    }

    static synchronized void refund(long n) {
        if (used >= n) used -= n;
    }

    private static long size(java.io.File f) {
        if (f.isFile()) return f.length();
        long t = 0;
        java.io.File[] l = f.listFiles();
        if (l != null) for (int i = 0; i < l.length; i++) t += size(l[i]);
        return t;
    }
}
