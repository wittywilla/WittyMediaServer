package javax.tv.service.transport;
public class ServiceDetailsChangeEvent extends javax.tv.service.SIChangeEvent {
    public ServiceDetailsChangeEvent(Object source, javax.tv.service.SIChangeType t, javax.tv.service.navigation.ServiceDetails d) { super(source, t, d); }
    public javax.tv.service.navigation.ServiceDetails getServiceDetails() { return (javax.tv.service.navigation.ServiceDetails) getSIElement(); }
}
