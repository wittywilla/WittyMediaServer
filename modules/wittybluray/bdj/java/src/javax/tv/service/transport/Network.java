package javax.tv.service.transport;
public interface Network extends javax.tv.service.SIElement {
    String getName();
    int getNetworkID();
    javax.tv.service.SIRequest retrieveTransportStreams(javax.tv.service.SIRequestor r);
}
