package org.dvb.user;
public class UnsupportedPreferenceException extends Exception {
    private final String preference;
    public UnsupportedPreferenceException() { preference = null; }
    public UnsupportedPreferenceException(String m, String preference) { super(m); this.preference = preference; }
    public String getPreference() { return preference; }
}
