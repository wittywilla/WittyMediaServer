package org.dvb.user;
public class UserPreferenceManager {
    private static final UserPreferenceManager it = new UserPreferenceManager();
    private UserPreferenceManager() {}
    public static UserPreferenceManager getInstance() { return it; }
    public void read(Preference p) {
        int v = 0;
        if ("User Language".equals(p.getName())) v = witty.bdj.Host.PSR[18];
        if (v != 0) p.add(new String(new char[] {(char) (v >> 16 & 255), (char) (v >> 8 & 255), (char) (v & 255)}));
    }
    public void read(Preference p, Object facility) { read(p); }
    public void write(Preference p) {}
    public void addUserPreferenceChangeListener(Object l) {}
    public void removeUserPreferenceChangeListener(Object l) {}
}
