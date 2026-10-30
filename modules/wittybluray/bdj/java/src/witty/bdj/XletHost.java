package witty.bdj;

import java.awt.Container;
import java.util.ArrayList;
import java.util.List;

/** Runs the Xlet life cycle, for either of the two Xlet interfaces discs use. */
public final class XletHost {
    private XletHost() {}

    static final class Ctx implements javax.tv.xlet.XletContext, javax.microedition.xlet.XletContext {
        final Apps.App app;
        Ctx(Apps.App a) { app = a; }
        public void notifyDestroyed() { app.state = 4; Host.send("appdestroyed " + app.id); }
        public void notifyPaused() { app.state = 2; }
        public void resumeRequest() {
            if (app.state != 2) return;
            final Object x = app.xlet;
            new Thread(app.group, new Runnable() {
                public void run() {
                    try {
                        if (x instanceof javax.tv.xlet.Xlet) ((javax.tv.xlet.Xlet) x).startXlet();
                        else ((javax.microedition.xlet.Xlet) x).startXlet();
                        app.state = 3;
                    } catch (Throwable t) {
                        Host.log("resume: " + t);
                    }
                }
            }).start();
        }
        public Object getXletProperty(String k) {
            if (javax.tv.xlet.XletContext.ARGS.equals(k) || "javax.microedition.xlet.args".equals(k)) return app.params;
            if ("dvb.org.id".equals(k)) return Integer.toHexString(app.org);
            if ("dvb.app.id".equals(k)) return Integer.toHexString(app.id);
            if ("dvb.caller.parameters".equals(k)) return new String[0];
            return null;
        }
        public Container getContainer() { return org.havi.ui.HSceneFactory.getInstance().getDefaultHScene(); }
    }

    static void run(Apps.App a, Object x) throws Exception {
        Ctx c = new Ctx(a);
        a.context = c;
        if (x instanceof javax.tv.xlet.Xlet) {
            javax.tv.xlet.Xlet v = (javax.tv.xlet.Xlet) x;
            v.initXlet(c);
            a.state = 2;
            v.startXlet();
        } else if (x instanceof javax.microedition.xlet.Xlet) {
            javax.microedition.xlet.Xlet v = (javax.microedition.xlet.Xlet) x;
            v.initXlet(c);
            a.state = 2;
            v.startXlet();
        } else {
            throw new ClassCastException(a.clazz + " is not an Xlet");
        }
    }

    static void destroy(Apps.App a) throws Exception {
        Object x = a.xlet;
        if (x instanceof javax.tv.xlet.Xlet) ((javax.tv.xlet.Xlet) x).destroyXlet(true);
        else if (x instanceof javax.microedition.xlet.Xlet) ((javax.microedition.xlet.Xlet) x).destroyXlet(true);
    }

    /** The application the current thread belongs to, or null. */
    public static Apps.App current() {
        ThreadGroup g = Thread.currentThread().getThreadGroup();
        synchronized (Apps.table) {
            for (Apps.App a : Apps.table) {
                for (ThreadGroup t = g; t != null; t = t.getParent()) if (t == a.group) return a;
            }
        }
        return null;
    }
}
