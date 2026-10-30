package witty.shim;

import java.io.IOException;

public class RandomAccessFile extends java.io.RandomAccessFile {
    private final boolean writing;
    public RandomAccessFile(String path, String mode) throws java.io.FileNotFoundException { super(check(path, mode), mode); writing = mode.indexOf('w') >= 0; }
    public RandomAccessFile(File f, String mode) throws java.io.FileNotFoundException { this(f.getPath(), mode); }
    private static java.io.File check(String p, String mode) throws java.io.FileNotFoundException {
        try {
            return Roots.resolve(p, mode.indexOf('w') >= 0);
        } catch (java.io.FileNotFoundException e) {
            throw e;
        } catch (IOException e) {
            throw new java.io.FileNotFoundException(p);
        }
    }
    public void write(int b) throws IOException { Roots.charge(1); super.write(b); }
    public void write(byte[] b, int o, int l) throws IOException { Roots.charge(l); super.write(b, o, l); }
}
