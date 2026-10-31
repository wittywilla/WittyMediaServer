package witty.bdj;

import java.io.File;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * The running applications of a title. The server names them: "app" lines for a
 * title's table, then "start". An application's life is the Xlet life cycle, run
 * on a thread of its own, so a disc that hangs in initXlet hangs only itself.
 */
public final class Apps {
    private Apps() {}

    /** One application from the disc's table. */
    public static final class App {
        public int org, id, control, priority;
        public String clazz, name = "", base;
        public String[] params = new String[0];
        public Object xlet;
        public Object context;
        public volatile int state; // 0 not loaded, 1 loaded, 2 paused, 3 started, 4 destroyed
        private volatile boolean stopped;
        /** Over for good: its own threads can still set the state back while they run on. */
        public boolean ended() { return stopped || state == 4; }
        public ThreadGroup group;
    }

    public static final List<App> table = new ArrayList<App>();
    public static DiscLoader loader;
    private static final Map<String, Boolean> jarsAdded = new HashMap<String, Boolean>();
    public static int title = 0xffff;

    static void init() {
        loader = new DiscLoader(Apps.class.getClassLoader());
        Host.listen(new Host.Listener() {
            public void onMessage(String[] w) {
                if (w[0].equals("app")) add(w);
                else if (w[0].equals("cache")) addJar(w[1]);
                else if (w[0].equals("start")) startAll();
                else if (w[0].equals("stopapps")) stopAll();
                else if (w[0].equals("title")) title = Integer.parseInt(w[1]);
            }
        });
    }

    /** app org id control priority base class param... (words %-escaped) */
    private static void add(String[] w) {
        App a = new App();
        a.org = (int) Long.parseLong(w[1]);
        a.id = Integer.parseInt(w[2]);
        a.control = Integer.parseInt(w[3]);
        a.priority = Integer.parseInt(w[4]);
        a.base = w[5];
        a.clazz = unescape(w[6]);
        a.params = new String[w.length - 7];
        for (int i = 7; i < w.length; i++) a.params[i - 7] = unescape(w[i]);
        synchronized (table) {
            table.add(a);
        }
        addJar(a.base);
    }

    static String unescape(String s) {
        try {
            return java.net.URLDecoder.decode(s, "UTF-8");
        } catch (Exception e) {
            return s;
        }
    }

    private static void addJar(String base) {
        if (base == null || base.length() == 0 || jarsAdded.containsKey(base)) return;
        jarsAdded.put(base, Boolean.TRUE);
        File f = new File(new File(Host.vfsRoot, "BDMV/JAR"), base + ".jar");
        if (!f.isFile()) f = new File(new File(Host.vfsRoot, "BDMV/JAR"), base);
        try {
            if (f.exists()) loader.addJar(f);
            else Host.log("missing JAR " + base);
        } catch (Exception e) {
            Host.log("JAR " + base + ": " + e);
        }
    }

    private static void startAll() {
        List<App> list;
        synchronized (table) {
            list = new ArrayList<App>(table);
        }
        for (App a : list) if (a.control == 1 && a.state == 0) start(a);
    }

    /** Starts one application; also the way AppProxy.start comes in. */
    public static void start(final App a) {
        // An application's home in the disc's storage: dvb.persistent.root/<org>/<app>, in hex.
        new File(Host.persistRoot, Integer.toHexString(a.org) + File.separator + Integer.toHexString(a.id)).mkdirs();
        new File(Host.persistRoot, "buda").mkdirs();
        a.stopped = false;
        a.group = new AppGroup(a);
        a.state = 1;
        Thread t = new Thread(a.group, new Runnable() {
            public void run() {
                try {
                    Class<?> c = loader.loadClass(a.clazz);
                    Object x = c.newInstance();
                    a.xlet = x;
                    if (a.ended()) return;
                    XletHost.run(a, x);
                    if (a.ended()) return;
                    a.state = 3;
                    Host.send("appstarted " + a.id);
                } catch (Throwable e) {
                    a.state = 4;
                    Host.send("appfailed " + a.id + " " + e.toString().replace('\n', ' '));
                    e.printStackTrace();
                }
            }
        }, "xlet-" + a.id);
        t.start();
    }

    private static final class AppGroup extends ThreadGroup {
        final App app;
        AppGroup(App a) { super("app-" + a.id); app = a; }
    }

    /** The application a thread belongs to, even after it ended; on a shared thread such as the event queue, the first one running. */
    public static App owner() {
        for (ThreadGroup g = Thread.currentThread().getThreadGroup(); g != null; g = g.getParent()) if (g instanceof AppGroup) return ((AppGroup) g).app;
        synchronized (table) {
            for (App a : table) if (!a.ended()) return a;
        }
        return null;
    }

    /** Ends the applications of a title. */
    public static void stopAll() {
        List<App> list;
        synchronized (table) {
            list = new ArrayList<App>(table);
            table.clear();
        }
        for (App a : list) stop(a);
    }

    public static void stop(final App a) {
        final boolean running = a.xlet != null && a.state != 4;
        a.stopped = true;
        a.state = 4;
        Plane.invalidate();
        Thread t = new Thread(new Runnable() {
            public void run() {
                try {
                    if (running) XletHost.destroy(a);
                } catch (Throwable e) {
                    Host.log("destroy: " + e);
                }
            }
        }, "xlet-end");
        t.setDaemon(true);
        t.start();
    }
}
