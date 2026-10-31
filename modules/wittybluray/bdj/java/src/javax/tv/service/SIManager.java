package javax.tv.service;
import javax.tv.locator.InvalidLocatorException;
import javax.tv.locator.Locator;
import javax.tv.service.navigation.ServiceFilter;
import javax.tv.service.navigation.ServiceList;
import javax.tv.service.navigation.ServiceListImpl;
import javax.tv.service.transport.Transport;

/** A disc player knows its titles and nothing else: titles resolve by locator, everything else is empty or fails. */
public abstract class SIManager {
    private static final SIManager it = new Impl();
    protected SIManager() {}
    public static SIManager createInstance() { return it; }
    public abstract Service getService(Locator l) throws InvalidLocatorException, SecurityException;
    public abstract ServiceList filterServices(ServiceFilter f);
    public abstract SIRequest retrieveSIElement(Locator l, SIRequestor r) throws InvalidLocatorException, SecurityException;
    public abstract SIRequest retrieveProgramEvent(Locator l, SIRequestor r) throws InvalidLocatorException, SecurityException;
    public abstract void setPreferredLanguage(String language);
    public abstract String getPreferredLanguage();
    public abstract void registerInterest(Locator l, boolean active) throws InvalidLocatorException, SecurityException;
    public abstract Transport[] getTransports();

    private static final class Impl extends SIManager {
        private volatile String language = "eng";

        private static org.bluray.ti.Title title(Locator l) {
            org.bluray.net.BDLocator b = null;
            if (l instanceof org.bluray.net.BDLocator) b = (org.bluray.net.BDLocator) l;
            else if (l != null && l.toExternalForm() != null && l.toExternalForm().startsWith("bd:")) {
                try {
                    b = new org.bluray.net.BDLocator(l.toExternalForm());
                } catch (Exception e) {
                    return null;
                }
            }
            return b == null || b.getTitleNumber() < 0 ? null : new org.bluray.ti.TitleImpl(b.getTitleNumber());
        }
        public Service getService(Locator l) throws InvalidLocatorException {
            org.bluray.ti.Title t = title(l);
            if (t == null) throw new InvalidLocatorException(l == null ? "no locator" : "not a service: " + l.toExternalForm());
            return t;
        }
        public ServiceList filterServices(ServiceFilter f) { return new ServiceListImpl(new Service[0]); }
        public SIRequest retrieveSIElement(Locator l, SIRequestor r) throws InvalidLocatorException {
            if (l == null) throw new InvalidLocatorException("no locator");
            org.bluray.ti.Title t = title(l);
            if (t == null) return SIDelivery.failure(r, SIRequestFailureType.DATA_UNAVAILABLE);
            return SIDelivery.success(r, new SIRetrievable[] {t});
        }
        public SIRequest retrieveProgramEvent(Locator l, SIRequestor r) throws InvalidLocatorException {
            if (l == null) throw new InvalidLocatorException("no locator");
            return SIDelivery.failure(r, SIRequestFailureType.DATA_UNAVAILABLE);
        }
        public void setPreferredLanguage(String s) { if (s != null && s.length() == 3) language = s; }
        public String getPreferredLanguage() { return language; }
        public void registerInterest(Locator l, boolean active) {}
        public Transport[] getTransports() { return new Transport[0]; }
    }
}
