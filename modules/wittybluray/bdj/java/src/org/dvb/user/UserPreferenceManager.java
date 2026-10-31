package org.dvb.user;

import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;

/** Preferences held for the life of the player; language and country start from the player settings. */
public class UserPreferenceManager {
    private static final UserPreferenceManager it = new UserPreferenceManager();
    private final java.util.Map<String, String[]> stored = new java.util.HashMap<String, String[]>();
    private final List<UserPreferenceChangeListener> listeners = new CopyOnWriteArrayList<UserPreferenceChangeListener>();
    private UserPreferenceManager() {}
    public static UserPreferenceManager getInstance() { return it; }

    private static String code(int v, int bytes) {
        char[] c = new char[bytes];
        for (int i = 0; i < bytes; i++) c[i] = (char) (v >> 8 * (bytes - 1 - i) & 255);
        return new String(c);
    }
    public void read(Preference p) throws UnsupportedPreferenceException {
        if (p == null) throw new NullPointerException();
        String[] have;
        synchronized (stored) { have = stored.get(p.getName()); }
        p.removeAll();
        if (have != null) {
            p.add(have);
            return;
        }
        int v = 0;
        if ("User Language".equals(p.getName())) v = witty.bdj.Host.PSR[18];
        if (v != 0) p.add(code(v, 3));
        else if ("Country Code".equals(p.getName()) && witty.bdj.Host.PSR[19] != 0) p.add(code(witty.bdj.Host.PSR[19], 2));
    }
    public void read(Preference p, Facility f) throws UnsupportedPreferenceException { read(p); }
    public void write(Preference p) throws UnsupportedPreferenceException, java.io.IOException {
        if (p == null) throw new NullPointerException();
        synchronized (stored) { stored.put(p.getName(), p.getFavourites()); }
        final UserPreferenceChangeEvent e = new UserPreferenceChangeEvent(this, p.getName());
        if (listeners.isEmpty()) return;
        Thread t = new Thread(new Runnable() {
            public void run() { for (UserPreferenceChangeListener l : listeners) { try { l.receiveUserPreferenceChangeEvent(e); } catch (Throwable x) {} } }
        }, "pref-change");
        t.setDaemon(true);
        t.start();
    }
    public void write(Preference p, Facility f) throws UnsupportedPreferenceException, java.io.IOException { write(p); }
    public void addUserPreferenceChangeListener(UserPreferenceChangeListener l) { if (l != null && !listeners.contains(l)) listeners.add(l); }
    public void removeUserPreferenceChangeListener(UserPreferenceChangeListener l) { listeners.remove(l); }
}
