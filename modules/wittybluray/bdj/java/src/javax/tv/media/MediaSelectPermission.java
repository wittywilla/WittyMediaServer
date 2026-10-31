package javax.tv.media;
public class MediaSelectPermission extends java.security.Permission {
    private final String actions;
    public MediaSelectPermission(String locator, String actions) { super(locator == null ? "" : locator); this.actions = actions == null ? "" : actions; }
    public MediaSelectPermission(javax.tv.locator.Locator l, String actions) { this(l == null ? "" : l.toExternalForm(), actions); }
    public boolean implies(java.security.Permission p) {
        if (!(p instanceof MediaSelectPermission)) return false;
        String n = getName(), m = p.getName();
        return (n.equals("*") || n.equals(m) || (n.endsWith("*") && m.startsWith(n.substring(0, n.length() - 1)))) && (actions.equals("*") || actions.equals(((MediaSelectPermission) p).actions));
    }
    public boolean equals(Object o) { return o instanceof MediaSelectPermission && getName().equals(((MediaSelectPermission) o).getName()) && actions.equals(((MediaSelectPermission) o).actions); }
    public int hashCode() { return getName().hashCode() ^ actions.hashCode(); }
    public String getActions() { return actions; }
}
