package witty.shim;

import java.io.InputStream;
import witty.bdj.DiscLoader;

/** What disc code sees of its class loader: its own resources and classes, no more. */
public class Loader {
    final DiscLoader real;
    Loader(DiscLoader l) { real = l; }
    public Class<?> loadClass(String name) throws ClassNotFoundException { return real.loadClass(name); }
    public InputStream getResourceAsStream(String name) { return real.getResourceAsStream(name); }
    public URL getResource(String name) { return real.read(name) == null ? null : new URL(real, name); }
    public static InputStream getSystemResourceAsStream(String name) { return null; }
    public static URL getSystemResource(String name) { return null; }
    public static Loader getSystemClassLoader() { return Patch.loader(); }
    public Loader getParent() { return null; }
}
