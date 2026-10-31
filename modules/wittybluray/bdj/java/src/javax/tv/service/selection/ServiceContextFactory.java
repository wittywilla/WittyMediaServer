package javax.tv.service.selection;

import java.util.ArrayList;
import java.util.List;
import witty.bdj.Host;

/** The one title context of a disc player. Selecting a title goes to the server, which runs it. */
public abstract class ServiceContextFactory {
    private static final Impl context = new Impl();
    private static final ServiceContextFactory it = new ServiceContextFactory() {};
    public static ServiceContextFactory getInstance() { return it; }
    public ServiceContext createServiceContext() throws InsufficientResourcesException, SecurityException { return context; }
    public ServiceContext getServiceContext(javax.tv.xlet.XletContext c) throws SecurityException, ServiceContextException { return context; }
    public ServiceContext[] getServiceContexts() { return new ServiceContext[] {context}; }

    private static final class Impl implements org.bluray.ti.selection.TitleContext {
        private final List<ServiceContextListener> ls = new java.util.concurrent.CopyOnWriteArrayList<ServiceContextListener>();
        public void start(org.bluray.ti.Title t, boolean restart) { select(t); }
        public void select(javax.tv.service.Service s) {
            if (s instanceof org.bluray.ti.TitleImpl) Host.send("title " + ((org.bluray.ti.TitleImpl) s).getTitleNumber());
            fire();
        }
        public void select(javax.tv.locator.Locator[] l) {
            if (l != null && l.length > 0 && l[0] instanceof org.bluray.net.BDLocator) {
                org.bluray.net.BDLocator b = (org.bluray.net.BDLocator) l[0];
                if (b.getTitleNumber() >= 0) Host.send("title " + b.getTitleNumber());
            }
            fire();
        }
        private void fire() {
            ServiceContextEvent e = new NormalContentEvent(this);
            for (ServiceContextListener c : ls) c.receiveServiceContextEvent(e);
        }
        public void stop() {}
        public void destroy() {}
        public ServiceContentHandler[] getServiceContentHandlers() { return new ServiceContentHandler[0]; }
        public javax.tv.service.Service getService() { return new org.bluray.ti.TitleImpl(witty.bdj.Apps.title); }
        public void addListener(ServiceContextListener l) { ls.add(l); }
        public void removeListener(ServiceContextListener l) { ls.remove(l); }
    }
}
