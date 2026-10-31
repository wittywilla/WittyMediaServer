package javax.tv.service.navigation;
public class ServiceComponentChangeEvent extends javax.tv.service.SIChangeEvent {
    public ServiceComponentChangeEvent(Object source, javax.tv.service.SIChangeType t, ServiceComponent c) { super(source, t, c); }
    public ServiceComponent getServiceComponent() { return (ServiceComponent) getSIElement(); }
}
