package javax.tv.service.navigation;
public abstract class ServiceFilter {
    protected ServiceFilter() {}
    public abstract boolean accept(javax.tv.service.Service s);
}
