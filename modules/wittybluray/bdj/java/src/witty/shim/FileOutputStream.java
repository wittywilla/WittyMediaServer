package witty.shim;

import java.io.IOException;

public class FileOutputStream extends java.io.FileOutputStream {
    public FileOutputStream(String path) throws java.io.FileNotFoundException { super(check(path)); }
    public FileOutputStream(String path, boolean append) throws java.io.FileNotFoundException { super(check(path), append); }
    public FileOutputStream(File f) throws java.io.FileNotFoundException { super(check(f.getPath())); }
    public FileOutputStream(File f, boolean append) throws java.io.FileNotFoundException { super(check(f.getPath()), append); }
    static java.io.File check(String p) throws java.io.FileNotFoundException {
        try {
            return Roots.resolve(p, true);
        } catch (java.io.FileNotFoundException e) {
            throw e;
        } catch (IOException e) {
            throw new java.io.FileNotFoundException(p);
        }
    }
    public void write(int b) throws IOException { Roots.charge(1); super.write(b); }
    public void write(byte[] b) throws IOException { Roots.charge(b.length); super.write(b); }
    public void write(byte[] b, int o, int l) throws IOException { Roots.charge(l); super.write(b, o, l); }
}
