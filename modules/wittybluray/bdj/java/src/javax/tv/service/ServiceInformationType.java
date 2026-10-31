package javax.tv.service;
public class ServiceInformationType {
    public static final ServiceInformationType ATSC_PSIP = new ServiceInformationType("ATSC_PSIP");
    public static final ServiceInformationType DVB_SI = new ServiceInformationType("DVB_SI");
    public static final ServiceInformationType SCTE_SI = new ServiceInformationType("SCTE_SI");
    public static final ServiceInformationType UNKNOWN = new ServiceInformationType("UNKNOWN");
    private final String n;
    protected ServiceInformationType(String s) { n = s; }
    public String toString() { return n; }
}
