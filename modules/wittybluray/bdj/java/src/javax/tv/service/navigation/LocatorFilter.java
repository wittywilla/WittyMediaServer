package javax.tv.service.navigation;
public class LocatorFilter extends ServiceFilter {
    private final javax.tv.locator.Locator[] f;
    public LocatorFilter(javax.tv.locator.Locator[] locators) { f = locators == null ? new javax.tv.locator.Locator[0] : locators.clone(); }
    public javax.tv.locator.Locator[] getFilterValue() { return f.clone(); }
    public boolean accept(javax.tv.service.Service s) {
        if (s == null || s.getLocator() == null) return false;
        for (javax.tv.locator.Locator l : f) if (l != null && l.toExternalForm().equals(s.getLocator().toExternalForm())) return true;
        return false;
    }
}
