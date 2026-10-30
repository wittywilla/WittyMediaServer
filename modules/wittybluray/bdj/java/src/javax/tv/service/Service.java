package javax.tv.service;
public interface Service {
    String getName();
    javax.tv.locator.Locator getLocator();
    boolean hasMultipleInstances();
    ServiceType getServiceType();
}
