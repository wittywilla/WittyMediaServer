package javax.tv.service.transport;
public interface NetworkCollection extends Transport {
    javax.tv.service.SIRequest retrieveNetworks(javax.tv.service.SIRequestor r);
    void addNetworkChangeListener(NetworkChangeListener l);
    void removeNetworkChangeListener(NetworkChangeListener l);
}
