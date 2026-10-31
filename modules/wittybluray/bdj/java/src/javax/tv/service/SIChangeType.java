package javax.tv.service;
public class SIChangeType {
    public static final SIChangeType ADD = new SIChangeType("ADD");
    public static final SIChangeType REMOVE = new SIChangeType("REMOVE");
    public static final SIChangeType MODIFY = new SIChangeType("MODIFY");
    private final String n;
    protected SIChangeType(String s) { n = s; }
    public String toString() { return n; }
}
