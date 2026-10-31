package org.dvb.application;

import java.util.ArrayList;
import java.util.Enumeration;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CopyOnWriteArrayList;
import witty.bdj.Apps;

/** The applications of the running title. */
public class AppsDatabase {
    private static final AppsDatabase it = new AppsDatabase();
    private static final Map<Apps.App, List<AppStateChangeEventListener>> watchers = new HashMap<Apps.App, List<AppStateChangeEventListener>>();
    private final List<AppsDatabaseEventListener> listeners = new CopyOnWriteArrayList<AppsDatabaseEventListener>();
    protected AppsDatabase() {}
    public static AppsDatabase getAppsDatabase() { return it; }
    private static List<Apps.App> all() { synchronized (Apps.table) { return new ArrayList<Apps.App>(Apps.table); } }
    private static Apps.App find(AppID id) { if (id == null) return null; for (Apps.App a : all()) if (a.org == id.getOID() && a.id == id.getAID()) return a; return null; }
    static boolean running(AppID id) { Apps.App a = find(id); return a != null && (a.state == 2 || a.state == 3); }
    public int size() { return all().size(); }
    public Enumeration<AppAttributes> getAppAttributes(AppsDatabaseFilter f) {
        List<AppAttributes> o = new ArrayList<AppAttributes>();
        for (Apps.App a : all()) if (f == null || f.accept(new AppID(a.org, a.id))) o.add(new Attrs(a));
        return java.util.Collections.enumeration(o);
    }
    public Enumeration<AppID> getAppIDs(AppsDatabaseFilter f) {
        List<AppID> o = new ArrayList<AppID>();
        for (Apps.App a : all()) if (f == null || f.accept(new AppID(a.org, a.id))) o.add(new AppID(a.org, a.id));
        return java.util.Collections.enumeration(o);
    }
    public AppAttributes getAppAttributes(AppID id) { Apps.App a = find(id); return a == null ? null : new Attrs(a); }
    public AppProxy getAppProxy(AppID id) { Apps.App a = find(id); return a == null ? null : new Proxy(a); }
    public void addListener(AppsDatabaseEventListener l) { if (l != null && !listeners.contains(l)) listeners.add(l); }
    public void removeListener(AppsDatabaseEventListener l) { listeners.remove(l); }

    private static final class Attrs implements AppAttributes {
        final Apps.App a;
        Attrs(Apps.App x) { a = x; }
        public int getType() { return BD_J_application; }
        public String getName() { return a.name; }
        public String getName(String l) { return a.name; }
        public String[][] getNames() { return new String[][] {{"eng", a.name}}; }
        public String[] getProfiles() { return new String[] {"bluray"}; }
        public int[] getVersions(String p) { return new int[] {2, 0, 0}; }
        public boolean getIsServiceBound() { return true; }
        public boolean isStartable() { return a.state == 0; }
        public AppID getIdentifier() { return new AppID(a.org, a.id); }
        public int getPriority() { return a.priority; }
        public javax.tv.locator.Locator getServiceLocator() { return null; }
        public Object getProperty(String k) { return null; }
        public boolean isVisible() { return true; }
        public AppIcon getAppIcon() { return null; }
    }

    private static int map(int s) {
        switch (s) {
            case 2: return AppProxy.PAUSED;
            case 3: return AppProxy.STARTED;
            case 4: return AppProxy.DESTROYED;
            default: return AppProxy.NOT_LOADED;
        }
    }

    private static final class Proxy implements DVBJProxy {
        final Apps.App a;
        Proxy(Apps.App x) { a = x; }
        public int getState() { return map(a.state); }
        public void start() { watch(); if (a.state == 0 || a.state == 4) Apps.start(a); }
        public void start(String[] args) { a.params = args == null ? new String[0] : args.clone(); start(); }
        public void stop(boolean forced) { watch(); Apps.stop(a); }
        public void pause() {
            final Object x = a.xlet;
            if (a.state != 3 || x == null) return;
            watch();
            run(new Runnable() {
                public void run() {
                    try {
                        if (x instanceof javax.tv.xlet.Xlet) ((javax.tv.xlet.Xlet) x).pauseXlet();
                        else if (x instanceof javax.microedition.xlet.Xlet) ((javax.microedition.xlet.Xlet) x).pauseXlet();
                        a.state = 2;
                    } catch (Throwable t) {}
                }
            });
        }
        public void resume() {
            final Object x = a.xlet;
            if (a.state != 2 || x == null) return;
            watch();
            run(new Runnable() {
                public void run() {
                    try {
                        if (x instanceof javax.tv.xlet.Xlet) ((javax.tv.xlet.Xlet) x).startXlet();
                        else if (x instanceof javax.microedition.xlet.Xlet) ((javax.microedition.xlet.Xlet) x).startXlet();
                        a.state = 3;
                    } catch (Throwable t) {}
                }
            });
        }
        private void run(Runnable r) {
            Thread t = a.group != null ? new Thread(a.group, r, "xlet-state") : new Thread(r, "xlet-state");
            t.setDaemon(true);
            t.start();
        }
        public void addAppStateChangeEventListener(AppStateChangeEventListener l) {
            if (l == null) return;
            synchronized (watchers) {
                List<AppStateChangeEventListener> v = watchers.get(a);
                if (v == null) watchers.put(a, v = new CopyOnWriteArrayList<AppStateChangeEventListener>());
                if (!v.contains(l)) v.add(l);
            }
        }
        public void removeAppStateChangeEventListener(AppStateChangeEventListener l) {
            synchronized (watchers) {
                List<AppStateChangeEventListener> v = watchers.get(a);
                if (v != null) {
                    v.remove(l);
                    if (v.isEmpty()) watchers.remove(a);
                }
            }
        }
        /** Tells the listeners of each change in the next few seconds, from a thread of its own. */
        private void watch() {
            final List<AppStateChangeEventListener> v;
            synchronized (watchers) { v = watchers.get(a); }
            if (v == null) return;
            Thread t = new Thread(new Runnable() {
                public void run() {
                    int last = map(a.state);
                    long end = System.currentTimeMillis() + 15000;
                    while (System.currentTimeMillis() < end) {
                        try { Thread.sleep(20); } catch (InterruptedException e) { return; }
                        int now = map(a.state);
                        if (now == last) continue;
                        AppStateChangeEvent e = new AppStateChangeEvent(new AppID(a.org, a.id), last, now, Proxy.this, false);
                        last = now;
                        for (AppStateChangeEventListener l : v) { try { l.stateChange(e); } catch (Throwable x) {} }
                        if (now == AppProxy.DESTROYED || now == AppProxy.STARTED) return;
                    }
                }
            }, "app-watch");
            t.setDaemon(true);
            t.start();
        }
    }
}
