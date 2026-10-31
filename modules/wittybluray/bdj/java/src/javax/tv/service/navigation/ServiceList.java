package javax.tv.service.navigation;
public interface ServiceList {
    ServiceList sortByName();
    ServiceList sortByNumber();
    ServiceIterator createServiceIterator();
    javax.tv.service.Service findService(javax.tv.locator.Locator l) throws javax.tv.locator.InvalidLocatorException;
    ServiceList filterServices(ServiceFilter f);
    boolean contains(javax.tv.service.Service s);
    int indexOf(javax.tv.service.Service s);
    int size();
    javax.tv.service.Service getService(int index);
}
