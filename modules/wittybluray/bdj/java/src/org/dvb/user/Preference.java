package org.dvb.user;
public abstract class Preference {
    private final String name;
    private final java.util.List<String> values = new java.util.ArrayList<String>();
    protected Preference(String n) { name = n; }
    public void add(String v) { values.add(v); }
    public void add(int position, String v) { values.remove(v); values.add(Math.max(0, Math.min(position, values.size())), v); }
    public void add(String[] v) { for (int i = 0; i < v.length; i++) values.add(v[i]); }
    public String[] getFavourites() { return values.toArray(new String[0]); }
    public String getMostFavourite() { return values.isEmpty() ? null : values.get(0); }
    public String getName() { return name; }
    public int getPosition(String v) { return values.indexOf(v); }
    public boolean hasValue() { return !values.isEmpty(); }
    public void remove(String v) { values.remove(v); }
    public void removeAll() { values.clear(); }
    public void setMostFavourite(String v) { values.remove(v); values.add(0, v); }
}
