package javax.tv.service.transport;
public class NetworkChangeEvent extends javax.tv.service.SIChangeEvent {
    public NetworkChangeEvent(Object source, javax.tv.service.SIChangeType t, Network n) { super(source, t, n); }
    public Network getNetwork() { return (Network) getSIElement(); }
}
