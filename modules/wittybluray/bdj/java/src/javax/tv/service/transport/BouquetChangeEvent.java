package javax.tv.service.transport;
public class BouquetChangeEvent extends javax.tv.service.SIChangeEvent {
    public BouquetChangeEvent(Object source, javax.tv.service.SIChangeType t, Bouquet b) { super(source, t, b); }
    public Bouquet getBouquet() { return (Bouquet) getSIElement(); }
}
