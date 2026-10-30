package witty.shim;

import java.io.IOException;

/** java.io.File for disc code: the same calls, confined to the disc's own areas. */
public class File implements java.io.Serializable, Comparable<File> {
    public static final char separatorChar = java.io.File.separatorChar;
    public static final String separator = java.io.File.separator;
    public static final char pathSeparatorChar = java.io.File.pathSeparatorChar;
    public static final String pathSeparator = java.io.File.pathSeparator;
    private final java.io.File f;

    public File(String path) { f = new java.io.File(path); }
    public File(String parent, String child) { f = new java.io.File(parent, child); }
    public File(File parent, String child) { f = new java.io.File(parent == null ? null : parent.f, child); }
    private File(java.io.File r) { f = r; }

    private boolean ok() { return Roots.known(f.getPath()); }
    public String getName() { return f.getName(); }
    public String getPath() { return f.getPath(); }
    public String getParent() { return f.getParent(); }
    public File getParentFile() { java.io.File p = f.getParentFile(); return p == null ? null : new File(p); }
    public boolean isAbsolute() { return f.isAbsolute(); }
    public String getAbsolutePath() { return f.getAbsolutePath(); }
    public File getAbsoluteFile() { return new File(f.getAbsoluteFile()); }
    public String getCanonicalPath() throws IOException { return f.getCanonicalPath(); }
    public File getCanonicalFile() throws IOException { return new File(f.getCanonicalFile()); }
    public boolean exists() { return ok() && f.exists(); }
    public boolean canRead() { return ok() && f.canRead(); }
    public boolean canWrite() { try { Roots.resolve(f.getPath(), true); return f.canWrite(); } catch (IOException e) { return false; } }
    public boolean isDirectory() { return ok() && f.isDirectory(); }
    public boolean isFile() { return ok() && f.isFile(); }
    public boolean isHidden() { return ok() && f.isHidden(); }
    public long lastModified() { return ok() ? f.lastModified() : 0; }
    public long length() { return ok() ? f.length() : 0; }
    public boolean createNewFile() throws IOException { Roots.resolve(f.getPath(), true); return f.createNewFile(); }
    public boolean delete() {
        try {
            Roots.resolve(f.getPath(), true);
            long n = f.isFile() ? f.length() : 0;
            boolean d = f.delete();
            if (d) Roots.refund(n);
            return d;
        } catch (IOException e) {
            return false;
        }
    }
    public void deleteOnExit() {}
    public String[] list() { return ok() ? f.list() : null; }
    public String[] list(FilenameFilter fl) {
        String[] all = list();
        if (all == null || fl == null) return all;
        java.util.ArrayList<String> o = new java.util.ArrayList<String>();
        for (int i = 0; i < all.length; i++) if (fl.accept(this, all[i])) o.add(all[i]);
        return o.toArray(new String[o.size()]);
    }
    public File[] listFiles() {
        String[] all = list();
        if (all == null) return null;
        File[] o = new File[all.length];
        for (int i = 0; i < all.length; i++) o[i] = new File(this, all[i]);
        return o;
    }
    public File[] listFiles(FilenameFilter fl) {
        String[] all = list(fl);
        if (all == null) return null;
        File[] o = new File[all.length];
        for (int i = 0; i < all.length; i++) o[i] = new File(this, all[i]);
        return o;
    }
    public File[] listFiles(FileFilter fl) {
        File[] all = listFiles();
        if (all == null || fl == null) return all;
        java.util.ArrayList<File> o = new java.util.ArrayList<File>();
        for (int i = 0; i < all.length; i++) if (fl.accept(all[i])) o.add(all[i]);
        return o.toArray(new File[o.size()]);
    }
    public boolean mkdir() { try { Roots.resolve(f.getPath(), true); return f.mkdir(); } catch (IOException e) { return false; } }
    public boolean mkdirs() { try { Roots.resolve(f.getPath(), true); return f.mkdirs(); } catch (IOException e) { return false; } }
    public boolean renameTo(File to) {
        try {
            Roots.resolve(f.getPath(), true);
            Roots.resolve(to.f.getPath(), true);
            return f.renameTo(to.f);
        } catch (IOException e) {
            return false;
        }
    }
    public boolean setLastModified(long t) { try { Roots.resolve(f.getPath(), true); return f.setLastModified(t); } catch (IOException e) { return false; } }
    public boolean setReadOnly() { try { Roots.resolve(f.getPath(), true); return f.setReadOnly(); } catch (IOException e) { return false; } }
    public URL toURL() throws java.net.MalformedURLException { return new URL("file:" + f.getAbsolutePath().replace('\\', '/')); }
    public int compareTo(File o) { return f.compareTo(o.f); }
    public boolean equals(Object o) { return o instanceof File && f.equals(((File) o).f); }
    public int hashCode() { return f.hashCode(); }
    public String toString() { return f.toString(); }
}
