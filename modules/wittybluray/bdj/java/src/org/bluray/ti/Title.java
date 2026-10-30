package org.bluray.ti;
public class Title implements javax.tv.service.Service {
    private final int n;
    public Title(int number) { n = number; }
    public int getTitleNumber() { return n; }
    public String getName() { return "Title " + n; }
    public javax.tv.locator.Locator getLocator() {
        try {
            return new org.bluray.net.BDLocator("bd://" + Integer.toHexString(n));
        } catch (org.davic.net.InvalidLocatorException e) {
            return null;
        }
    }
    public boolean hasMultipleInstances() { return false; }
    public javax.tv.service.ServiceType getServiceType() { return javax.tv.service.ServiceType.UNKNOWN; }
    public boolean equals(Object o) { return o instanceof Title && ((Title) o).n == n; }
    public int hashCode() { return n; }
}
