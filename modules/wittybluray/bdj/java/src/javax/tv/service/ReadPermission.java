package javax.tv.service;
public class ReadPermission extends java.security.Permission {
    private final String actions;
    public ReadPermission(String locator, String actions) { super(locator == null ? "" : locator); this.actions = actions == null ? "" : actions; }
    public ReadPermission(javax.tv.locator.Locator l, String actions) { this(l == null ? "" : l.toExternalForm(), actions); }
    public boolean implies(java.security.Permission p) {
        if (!(p instanceof ReadPermission)) return false;
        String n = getName(), m = p.getName();
        return (n.equals("*") || n.equals(m) || (n.endsWith("*") && m.startsWith(n.substring(0, n.length() - 1)))) && (actions.equals("*") || actions.equals(((ReadPermission) p).actions));
    }
    public boolean equals(Object o) { return o instanceof ReadPermission && getName().equals(((ReadPermission) o).getName()) && actions.equals(((ReadPermission) o).actions); }
    public int hashCode() { return getName().hashCode() ^ actions.hashCode(); }
    public String getActions() { return actions; }
}
