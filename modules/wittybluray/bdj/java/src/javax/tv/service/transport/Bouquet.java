package javax.tv.service.transport;
public interface Bouquet extends javax.tv.service.SIElement {
    String getName();
    int getBouquetID();
    javax.tv.service.SIRequest retrieveServiceDetails(javax.tv.service.SIRequestor r);
}
