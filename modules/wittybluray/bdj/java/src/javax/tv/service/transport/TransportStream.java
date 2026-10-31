package javax.tv.service.transport;
public interface TransportStream extends javax.tv.service.SIElement {
    String getDescription();
    int getTransportStreamID();
    javax.tv.service.SIRequest retrieveServiceDetails(javax.tv.service.SIRequestor r);
}
