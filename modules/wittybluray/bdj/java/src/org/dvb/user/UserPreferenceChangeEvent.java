package org.dvb.user;
public class UserPreferenceChangeEvent extends java.util.EventObject {
    private final String name;
    public UserPreferenceChangeEvent(Object source, String name) { super(source); this.name = name; }
    public String getName() { return name; }
}
