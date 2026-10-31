package javax.tv.service.navigation;
public class ServiceTypeFilter extends ServiceFilter {
    private final javax.tv.service.ServiceType t;
    public ServiceTypeFilter(javax.tv.service.ServiceType type) { t = type; }
    public javax.tv.service.ServiceType getFilterValue() { return t; }
    public boolean accept(javax.tv.service.Service s) { return s != null && t != null && t.equals(s.getServiceType()); }
}
