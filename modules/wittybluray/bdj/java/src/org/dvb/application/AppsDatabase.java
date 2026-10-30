package org.dvb.application;

import java.util.ArrayList;
import java.util.Enumeration;
import java.util.List;
import witty.bdj.Apps;

/** The applications of the running title. */
public class AppsDatabase {
    private static final AppsDatabase it = new AppsDatabase();
    protected AppsDatabase() {}
    public static AppsDatabase getAppsDatabase() { return it; }
    private static List<Apps.App> all() { synchronized (Apps.table) { return new ArrayList<Apps.App>(Apps.table); } }
    private static Apps.App find(AppID id) { for (Apps.App a : all()) if (a.org == id.getOID() && a.id == id.getAID()) return a; return null; }
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
    public void addListener(AppsDatabaseEventListener l) {}
    public void removeListener(AppsDatabaseEventListener l) {}

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
    }
    private static final class Proxy implements AppProxy {
        final Apps.App a;
        Proxy(Apps.App x) { a = x; }
        public int getState() { switch (a.state) { case 0: return NOT_LOADED; case 2: return PAUSED; case 3: return STARTED; case 4: return DESTROYED; default: return NOT_LOADED; } }
        public void start() { if (a.state == 0 || a.state == 4) Apps.start(a); }
        public void start(String[] args) { a.params = args; start(); }
        public void stop(boolean forced) { Apps.stop(a); }
        public void pause() {}
        public void resume() {}
        public void addAppStateChangeEventListener(AppStateChangeEventListener l) {}
        public void removeAppStateChangeEventListener(AppStateChangeEventListener l) {}
    }
}
