package org.bluray.storage;

import witty.bdj.Host;
import witty.shim.File;

/** A disc's storage: every area shares the one allowance the file classes enforce. */
public class DataArea {
    private static final long ALLOWANCE = 64L << 20;
    public String getRoot() { return Host.persistRoot; }
    public long getTotalSpace() { return ALLOWANCE; }
    public long getFreeSpace() { return Math.max(0, ALLOWANCE - size(new File(Host.persistRoot), 0) - size(new File(Host.tempRoot), 0)); }
    private static long size(File f, int depth) {
        if (f.isFile()) return f.length();
        File[] all = depth > 32 ? null : f.listFiles();
        long t = 0;
        if (all != null) for (int i = 0; i < all.length; i++) t += size(all[i], depth + 1);
        return t;
    }
    public void addListener(Object l) {}
    public void removeListener(Object l) {}
}
