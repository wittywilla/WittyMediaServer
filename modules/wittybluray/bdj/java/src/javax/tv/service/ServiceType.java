package javax.tv.service;
public class ServiceType {
    public static final ServiceType UNKNOWN = new ServiceType("UNKNOWN");
    private final String n;
    protected ServiceType(String s) { n = s; }
    public String toString() { return n; }
}
