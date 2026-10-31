package javax.tv.service;
public interface SIRetrievable {
    javax.tv.locator.Locator getLocator();
    ServiceInformationType getServiceInformationType();
    java.util.Date getUpdateTime();
}
