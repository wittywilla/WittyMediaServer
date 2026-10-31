package javax.tv.service.navigation;
public class DeliverySystemType {
    public static final DeliverySystemType CABLE = new DeliverySystemType("CABLE");
    public static final DeliverySystemType SATELLITE = new DeliverySystemType("SATELLITE");
    public static final DeliverySystemType TERRESTRIAL = new DeliverySystemType("TERRESTRIAL");
    public static final DeliverySystemType UNKNOWN = new DeliverySystemType("UNKNOWN");
    private final String n;
    protected DeliverySystemType(String s) { n = s; }
    public String toString() { return n; }
}
