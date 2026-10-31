package org.dvb.user;
public class Facility {
    private final String name;
    public Facility(String name) { this.name = name; }
    public String getName() { return name; }
    public boolean equals(Object o) { return o instanceof Facility && String.valueOf(((Facility) o).name).equals(String.valueOf(name)); }
    public int hashCode() { return name == null ? 0 : name.hashCode(); }
}
