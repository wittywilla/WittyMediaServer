package witty.bdj;

import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.IOException;
import java.io.InputStream;
import java.util.ArrayList;
import java.util.List;
import java.util.zip.ZipEntry;
import java.util.zip.ZipFile;

/** Loads a disc's classes from its JAR files, through the sandbox. */
public final class DiscLoader extends ClassLoader {
    private final List<ZipFile> jars = new ArrayList<ZipFile>();
    private final List<File> dirs = new ArrayList<File>();

    DiscLoader(ClassLoader parent) { super(parent); }

    void addJar(File f) throws IOException {
        if (f.isDirectory()) dirs.add(f); else jars.add(new ZipFile(f));
    }

    protected synchronized Class<?> loadClass(String name, boolean resolve) throws ClassNotFoundException {
        Class<?> c = findLoadedClass(name);
        if (c == null) {
            if (Sandbox.isPlatform(name)) {
                try {
                    c = getParent().loadClass(name);
                } catch (ClassNotFoundException e) {
                    c = define(name);
                }
            } else if (Sandbox.isReserved(name)) {
                if (!Sandbox.allowedJdk(name)) throw new ClassNotFoundException(name + " is not available to disc applications");
                c = getParent().loadClass(name);
            } else {
                // Any other non-disc name (org.w3c.dom and friends) goes through the same list.
                if (Sandbox.allowedJdk(name)) c = getParent().loadClass(name);
                else c = define(name);
            }
        }
        if (resolve) resolveClass(c);
        return c;
    }

    private Class<?> define(String name) throws ClassNotFoundException {
        if (Sandbox.isReserved(name)) throw new ClassNotFoundException(name);
        byte[] b = read(name.replace('.', '/') + ".class");
        if (b == null) throw new ClassNotFoundException(name);
        try {
            byte[] p = ClassPatch.patch(b);
            return defineClass(name, p, 0, p.length);
        } catch (RuntimeException e) {
            throw new ClassNotFoundException(name + ": damaged class file", e);
        }
    }

    /** A resource from the disc's JARs, or null. */
    public byte[] read(String path) {
        if (path.indexOf("..") >= 0) return null;
        while (path.startsWith("/")) path = path.substring(1);
        try {
            for (ZipFile z : jars) {
                ZipEntry e = z.getEntry(path);
                if (e != null) {
                    InputStream in = z.getInputStream(e);
                    try {
                        return drain(in);
                    } finally {
                        in.close();
                    }
                }
            }
            for (File d : dirs) {
                File f = new File(d, path);
                if (f.isFile()) {
                    InputStream in = new java.io.FileInputStream(f);
                    try {
                        return drain(in);
                    } finally {
                        in.close();
                    }
                }
            }
        } catch (IOException e) {
            return null;
        }
        return null;
    }

    private static byte[] drain(InputStream in) throws IOException {
        ByteArrayOutputStream o = new ByteArrayOutputStream();
        byte[] b = new byte[16384];
        for (int n; (n = in.read(b)) > 0; ) o.write(b, 0, n);
        return o.toByteArray();
    }

    public java.io.InputStream getResourceAsStream(String name) {
        byte[] b = read(name);
        return b == null ? null : new java.io.ByteArrayInputStream(b);
    }
}
