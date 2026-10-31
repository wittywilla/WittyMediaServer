package javax.tv.service.navigation;
import javax.tv.service.Service;

/** A fixed list of services (a platform helper; discs get lists from SIManager). */
public final class ServiceListImpl implements ServiceList {
    private final Service[] v;
    public ServiceListImpl(Service[] services) { v = services == null ? new Service[0] : services.clone(); }
    public ServiceList sortByName() {
        Service[] c = v.clone();
        java.util.Arrays.sort(c, new java.util.Comparator<Service>() {
            public int compare(Service a, Service b) { return String.valueOf(a.getName()).compareTo(String.valueOf(b.getName())); }
        });
        return new ServiceListImpl(c);
    }
    public ServiceList sortByNumber() {
        Service[] c = v.clone();
        java.util.Arrays.sort(c, new java.util.Comparator<Service>() {
            public int compare(Service a, Service b) { return num(a) - num(b); }
            private int num(Service s) { return s instanceof javax.tv.service.ServiceNumber ? ((javax.tv.service.ServiceNumber) s).getServiceNumber() : Integer.MAX_VALUE; }
        });
        return new ServiceListImpl(c);
    }
    public ServiceIterator createServiceIterator() {
        return new ServiceIterator() {
            private int i;
            public Service nextService() { return i < v.length ? v[i++] : null; }
            public Service previousService() { return i > 0 ? v[--i] : null; }
            public boolean hasNext() { return i < v.length; }
            public boolean hasPrevious() { return i > 0; }
            public void toBeginning() { i = 0; }
            public void toEnd() { i = v.length; }
        };
    }
    public Service findService(javax.tv.locator.Locator l) throws javax.tv.locator.InvalidLocatorException {
        if (l != null) for (Service s : v) if (s.getLocator() != null && l.toExternalForm().equals(s.getLocator().toExternalForm())) return s;
        throw new javax.tv.locator.InvalidLocatorException("no such service in the list");
    }
    public ServiceList filterServices(ServiceFilter f) {
        java.util.ArrayList<Service> o = new java.util.ArrayList<Service>();
        for (Service s : v) if (f == null || f.accept(s)) o.add(s);
        return new ServiceListImpl(o.toArray(new Service[0]));
    }
    public boolean contains(Service s) { return indexOf(s) >= 0; }
    public int indexOf(Service s) {
        for (int i = 0; i < v.length; i++) if (v[i].equals(s)) return i;
        return -1;
    }
    public int size() { return v.length; }
    public Service getService(int i) { return i >= 0 && i < v.length ? v[i] : null; }
}
