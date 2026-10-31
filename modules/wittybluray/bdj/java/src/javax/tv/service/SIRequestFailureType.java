package javax.tv.service;
public class SIRequestFailureType {
    public static final SIRequestFailureType DATA_UNAVAILABLE = new SIRequestFailureType("DATA_UNAVAILABLE");
    public static final SIRequestFailureType INSUFFICIENT_RESOURCES = new SIRequestFailureType("INSUFFICIENT_RESOURCES");
    public static final SIRequestFailureType LOCATOR_MALFORMED = new SIRequestFailureType("LOCATOR_MALFORMED");
    public static final SIRequestFailureType CANCELED = new SIRequestFailureType("CANCELED");
    private final String n;
    protected SIRequestFailureType(String s) { n = s; }
    public String toString() { return n; }
}
