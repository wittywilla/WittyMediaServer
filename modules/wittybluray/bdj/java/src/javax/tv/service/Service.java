package javax.tv.service;
public interface Service extends SIElement {
    String getName();
    javax.tv.locator.Locator getLocator();
    boolean hasMultipleInstances();
    ServiceType getServiceType();
    /** A disc player has no service details to hand out: the request fails. */
    default SIRequest retrieveDetails(SIRequestor r) { return SIDelivery.failure(r, SIRequestFailureType.DATA_UNAVAILABLE); }
    default ServiceInformationType getServiceInformationType() { return ServiceInformationType.UNKNOWN; }
    default java.util.Date getUpdateTime() { return new java.util.Date(0); }
}
