package javax.tv.service.navigation;
public class NavigationPermission extends java.security.Permission {
    private final String actions;
    public NavigationPermission(String name, String actions) { super(name == null ? "" : name); this.actions = actions == null ? "" : actions; }
    public boolean implies(java.security.Permission p) {
        if (!(p instanceof NavigationPermission)) return false;
        String n = getName(), m = p.getName();
        return (n.equals("*") || n.equals(m) || (n.endsWith("*") && m.startsWith(n.substring(0, n.length() - 1)))) && (actions.equals("*") || actions.equals(((NavigationPermission) p).actions));
    }
    public boolean equals(Object o) { return o instanceof NavigationPermission && getName().equals(((NavigationPermission) o).getName()) && actions.equals(((NavigationPermission) o).actions); }
    public int hashCode() { return getName().hashCode() ^ actions.hashCode(); }
    public String getActions() { return actions; }
}
