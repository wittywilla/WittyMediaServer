package witty.shim;

import java.io.IOException;

public class FileInputStream extends java.io.FileInputStream {
    public FileInputStream(String path) throws java.io.FileNotFoundException { super(check(path)); }
    public FileInputStream(File f) throws java.io.FileNotFoundException { super(check(f.getPath())); }
    private static java.io.File check(String p) throws java.io.FileNotFoundException {
        try {
            return Roots.resolve(p, false);
        } catch (java.io.FileNotFoundException e) {
            throw e;
        } catch (IOException e) {
            throw new java.io.FileNotFoundException(p);
        }
    }
}
