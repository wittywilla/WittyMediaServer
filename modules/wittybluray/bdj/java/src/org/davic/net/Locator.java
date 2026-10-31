package org.davic.net;
public class Locator implements javax.tv.locator.Locator {
    protected String url;
    public Locator(String u) { url = u; }
    public String toExternalForm() { return url; }
    public boolean hasMultipleTransformations() { return false; }
    public String toString() { return url; }
    public boolean equals(Object o) { return o instanceof Locator && String.valueOf(((Locator) o).url).equals(String.valueOf(url)); }
    public int hashCode() { return url == null ? 0 : url.hashCode(); }
}
