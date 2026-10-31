package javax.tv.service.transport;
public interface ServiceDetailsCollection extends Transport {
    javax.tv.service.SIRequest retrieveServiceDetails(javax.tv.service.SIRequestor r);
    void addServiceDetailsChangeListener(ServiceDetailsChangeListener l);
    void removeServiceDetailsChangeListener(ServiceDetailsChangeListener l);
}
