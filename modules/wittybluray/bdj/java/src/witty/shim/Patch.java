package witty.shim;

import java.awt.Component;
import witty.bdj.Apps;
import witty.bdj.DiscLoader;

/** The targets of the calls ClassPatch rewrites. */
public final class Patch {
    private Patch() {}
    private static Loader loader;

    static synchronized Loader loader() {
        if (loader == null) loader = new Loader(Apps.loader);
        return loader;
    }
    public static URL getResource(Class<?> c, String name) {
        String n = name;
        if (n.startsWith("/")) n = n.substring(1);
        else {
            String pkg = c.getName();
            int d = pkg.lastIndexOf('.');
            if (d > 0) n = pkg.substring(0, d).replace('.', '/') + "/" + n;
        }
        return Apps.loader.read(n) == null ? null : new URL(Apps.loader, n);
    }
    public static Loader getClassLoader(Class<?> c) { return c.getClassLoader() instanceof DiscLoader ? loader() : null; }
    public static Loader getContextClassLoader(Thread t) { return loader(); }
    public static Toolkit getToolkit(Component c) { return Toolkit.getDefaultToolkit(); }
    public static void printStackTrace(Object t, java.io.PrintStream s) { if (t instanceof Throwable) ((Throwable) t).printStackTrace(s); }
    public static void printStackTrace(Object t, java.io.PrintWriter w) { if (t instanceof Throwable) ((Throwable) t).printStackTrace(w); }
    public static void list(Object o, java.io.PrintStream s) {
        if (o instanceof java.util.Properties) ((java.util.Properties) o).list(s);
        else if (o instanceof java.awt.Component) ((java.awt.Component) o).list(s);
    }
    public static void list(Object o, java.io.PrintWriter w) {
        if (o instanceof java.util.Properties) ((java.util.Properties) o).list(w);
        else if (o instanceof java.awt.Component) ((java.awt.Component) o).list(w);
    }
}
