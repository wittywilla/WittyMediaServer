package javax.tv.service.selection;
public interface ServiceContext {
    void select(javax.tv.service.Service s) throws SecurityException;
    void select(javax.tv.locator.Locator[] l) throws javax.tv.locator.InvalidLocatorException, InvalidServiceComponentException, SecurityException;
    void stop() throws SecurityException;
    void destroy() throws SecurityException;
    ServiceContentHandler[] getServiceContentHandlers() throws SecurityException;
    javax.tv.service.Service getService();
    void addListener(ServiceContextListener l);
    void removeListener(ServiceContextListener l);
}
