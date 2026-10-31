package witty.shim;

import java.io.InputStream;
import witty.bdj.Apps;
import witty.bdj.DiscLoader;

/**
 * What disc code sees of its class loader: its own resources and classes, no more.
 * A disc's own subclass gets the same: every class it asks for comes through the
 * loader and the rules every disc class passes. There is no way to define a class.
 */
public class Loader {
    final DiscLoader real;
    Loader(DiscLoader l) { real = l; }
    protected Loader() { this(Apps.loader); }
    protected Loader(Loader parent) { this(Apps.loader); }
    public Class<?> loadClass(String name) throws ClassNotFoundException { return real.loadClass(name); }
    public InputStream getResourceAsStream(String name) { return real.getResourceAsStream(name); }
    public URL getResource(String name) { return real.read(name) == null ? null : new URL(real, name); }
    public static InputStream getSystemResourceAsStream(String name) { return null; }
    public static URL getSystemResource(String name) { return null; }
    public static Loader getSystemClassLoader() { return Patch.loader(); }
    public Loader getParent() { return null; }
    /** Adds what a disc's application names to the class path, if it is the disc's. */
    protected final void addDiscFile(String path) throws java.io.IOException {
        java.io.File f = Roots.resolve(path, false);
        if (f.exists()) real.addDiscFile(f.getCanonicalFile());
    }
}
