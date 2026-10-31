package javax.tv.service.selection;
public class ServiceContextPermission extends java.security.Permission {
    private final String actions;
    public ServiceContextPermission(String name, String actions) { super(name == null ? "" : name); this.actions = actions == null ? "" : actions; }
    public boolean implies(java.security.Permission p) {
        if (!(p instanceof ServiceContextPermission)) return false;
        String n = getName(), m = p.getName();
        return (n.equals("*") || n.equals(m) || (n.endsWith("*") && m.startsWith(n.substring(0, n.length() - 1)))) && (actions.equals("*") || actions.equals(((ServiceContextPermission) p).actions));
    }
    public boolean equals(Object o) { return o instanceof ServiceContextPermission && getName().equals(((ServiceContextPermission) o).getName()) && actions.equals(((ServiceContextPermission) o).actions); }
    public int hashCode() { return getName().hashCode() ^ actions.hashCode(); }
    public String getActions() { return actions; }
}
