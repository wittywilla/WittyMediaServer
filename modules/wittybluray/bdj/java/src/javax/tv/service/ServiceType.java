package javax.tv.service;
public class ServiceType {
    public static final ServiceType ANALOG_TV = new ServiceType("ANALOG_TV");
    public static final ServiceType DIGITAL_TV = new ServiceType("DIGITAL_TV");
    public static final ServiceType ANALOG_RADIO = new ServiceType("ANALOG_RADIO");
    public static final ServiceType DIGITAL_RADIO = new ServiceType("DIGITAL_RADIO");
    public static final ServiceType NVOD_REFERENCE = new ServiceType("NVOD_REFERENCE");
    public static final ServiceType NVOD_TIME_SHIFTED = new ServiceType("NVOD_TIME_SHIFTED");
    public static final ServiceType DATA_BROADCAST = new ServiceType("DATA_BROADCAST");
    public static final ServiceType DATA_APPLICATION = new ServiceType("DATA_APPLICATION");
    public static final ServiceType UNKNOWN = new ServiceType("UNKNOWN");
    private final String n;
    protected ServiceType(String s) { n = s; }
    public String toString() { return n; }
}
