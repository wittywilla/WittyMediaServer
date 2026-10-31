package javax.tv.service.transport;
public interface BouquetCollection extends Transport {
    javax.tv.service.SIRequest retrieveBouquets(javax.tv.service.SIRequestor r);
    void addBouquetChangeListener(BouquetChangeListener l);
    void removeBouquetChangeListener(BouquetChangeListener l);
}
