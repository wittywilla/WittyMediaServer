package witty.shim;

import java.io.IOException;

public class FileOutputStream extends java.io.FileOutputStream {
    public FileOutputStream(String path) throws java.io.FileNotFoundException { super(replacing(path)); }
    public FileOutputStream(String path, boolean append) throws java.io.FileNotFoundException { super(append ? check(path) : replacing(path), append); }
    public FileOutputStream(File f) throws java.io.FileNotFoundException { super(replacing(f.getPath())); }
    public FileOutputStream(File f, boolean append) throws java.io.FileNotFoundException { super(append ? check(f.getPath()) : replacing(f.getPath()), append); }
    static java.io.File check(String p) throws java.io.FileNotFoundException {
        try {
            return Roots.resolve(p, true);
        } catch (java.io.FileNotFoundException e) {
            throw e;
        } catch (IOException e) {
            throw new java.io.FileNotFoundException(p);
        }
    }
    /** Opening a file to write it anew gives back what it held. */
    private static java.io.File replacing(String p) throws java.io.FileNotFoundException {
        java.io.File f = check(p);
        if (f.isFile()) Roots.refund(f.length());
        return f;
    }
    public void write(int b) throws IOException { Roots.charge(1); super.write(b); }
    public void write(byte[] b) throws IOException { Roots.charge(b.length); super.write(b); }
    public void write(byte[] b, int o, int l) throws IOException { Roots.charge(l); super.write(b, o, l); }
}
