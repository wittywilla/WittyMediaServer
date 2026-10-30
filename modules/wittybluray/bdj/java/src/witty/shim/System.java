package witty.shim;

import java.util.Properties;
import witty.bdj.Host;

/** java.lang.System for disc code: its own properties, no exit, no native code. */
public final class System {
    private System() {}
    public static final PrintStream out = new PrintStream(java.lang.System.err, true);
    public static final PrintStream err = out;
    public static final java.io.InputStream in = new java.io.ByteArrayInputStream(new byte[0]);
    private static final Properties props = new Properties();
    static {
        String[][] p = {
            {"file.separator", java.io.File.separator}, {"path.separator", java.io.File.pathSeparator}, {"line.separator", "\n"},
            {"java.version", "1.4.2"}, {"java.vendor", "WittyBluray"}, {"java.specification.version", "1.3"}, {"java.vm.name", "CDC"},
            {"microedition.profiles", "PBP"}, {"microedition.configuration", "CDC"}, {"os.name", "BD-J"}, {"os.arch", "unknown"}, {"os.version", "1"},
            {"user.language", "en"}, {"user.region", "US"},
            {"bluray.vfs.root", Host.vfsRoot}, {"dvb.persistent.root", Host.persistRoot}, {"bluray.bindingunit.root", Host.persistRoot + java.io.File.separator + "buda"},
            {"java.io.tmpdir", Host.tempRoot}, {"user.dir", Host.vfsRoot},
            {"bluray.profile.1", "YES"}, {"bluray.profile.2", "YES"}, {"bluray.p2.version", "1.1"}, {"bluray.bd-j.version", "2.0"},
            {"bluray.localstorage.level", "5"}, {"bluray.localstorage.maxlevel", "5"}, {"bluray.disc.avplayback.readcapability", "YES"},
            {"havi.implementation.name", "WittyBluray"}, {"havi.implementation.vendor", "WittyBluray"}, {"havi.implementation.version", "1.1"},
            {"havi.specification.name", "HAVi"}, {"havi.specification.vendor", "HAVi"}, {"havi.specification.version", "1.1"},
            {"dvb.returnchannel.timeout", "0"}, {"mhp.profile.enhanced_broadcast", "NO"},
        };
        for (int i = 0; i < p.length; i++) props.setProperty(p[i][0], p[i][1]);
    }
    public static void arraycopy(Object a, int ai, Object b, int bi, int n) { java.lang.System.arraycopy(a, ai, b, bi, n); }
    public static long currentTimeMillis() { return java.lang.System.currentTimeMillis(); }
    public static long nanoTime() { return java.lang.System.nanoTime(); }
    public static int identityHashCode(Object o) { return java.lang.System.identityHashCode(o); }
    public static String getProperty(String k) { return props.getProperty(k); }
    public static String getProperty(String k, String d) { return props.getProperty(k, d); }
    public static String setProperty(String k, String v) { return (String) props.setProperty(k, v); }
    public static Properties getProperties() { return props; }
    public static String lineSeparator() { return "\n"; }
    public static String getenv(String k) { return null; }
    public static SecurityManager getSecurityManager() { return null; }
    public static void gc() { java.lang.System.gc(); }
    public static void runFinalization() {}
    public static void exit(int c) { Host.log("the disc asked to exit (" + c + "); ignored"); }
    public static void load(String f) { throw new UnsatisfiedLinkError("native code is not available to disc applications"); }
    public static void loadLibrary(String f) { throw new UnsatisfiedLinkError("native code is not available to disc applications"); }
}
