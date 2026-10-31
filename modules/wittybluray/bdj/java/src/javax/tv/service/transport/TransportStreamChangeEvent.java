package javax.tv.service.transport;
public class TransportStreamChangeEvent extends javax.tv.service.SIChangeEvent {
    public TransportStreamChangeEvent(Object source, javax.tv.service.SIChangeType t, TransportStream s) { super(source, t, s); }
    public TransportStream getTransportStream() { return (TransportStream) getSIElement(); }
}
