package org.dvb.io.persistent;

import java.io.FileNotFoundException;
import java.io.IOException;
import java.util.Date;

/** Priority, expiry and permissions of a stored file, kept in memory for the life of the player. */
public class FileAttributes {
    private static final java.util.Map<String, FileAttributes> store = new java.util.LinkedHashMap<String, FileAttributes>() {
        protected boolean removeEldestEntry(java.util.Map.Entry<String, FileAttributes> e) { return size() > 2048; }
    };
    private final Date expires;
    private final byte priority;
    private final FileAccessPermissions perms;

    public FileAttributes(Date expirationDate, byte priority, FileAccessPermissions permissions) {
        expires = expirationDate == null ? null : new Date(expirationDate.getTime());
        this.priority = priority;
        perms = permissions == null ? new FileAccessPermissions(true, true, true, true, true, true) : permissions;
    }
    public FileAttributes(FileAccessPermissions permissions, Date expirationDate, byte priority) { this(expirationDate, priority, permissions); }
    public Date getExpirationDate() { return expires == null ? null : new Date(expires.getTime()); }
    public byte getPriority() { return priority; }
    public FileAccessPermissions getPermissions() { return perms; }

    public static FileAttributes getFileAttributes(witty.shim.File f) throws IOException {
        if (f == null) throw new NullPointerException();
        if (!f.exists()) throw new FileNotFoundException(f.getName());
        synchronized (store) {
            FileAttributes a = store.get(f.getAbsolutePath());
            return a != null ? a : new FileAttributes(new Date(Long.MAX_VALUE), (byte) 0, null);
        }
    }
    public static void setFileAttributes(FileAttributes a, witty.shim.File f) throws IOException {
        if (a == null || f == null) throw new NullPointerException();
        if (!f.exists()) throw new FileNotFoundException(f.getName());
        synchronized (store) { store.put(f.getAbsolutePath(), a); }
    }
}
