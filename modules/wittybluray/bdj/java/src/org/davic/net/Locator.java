package org.davic.net;
public class Locator implements javax.tv.locator.Locator {
    protected String url;
    public Locator(String u) { url = u; }
    public String toExternalForm() { return url; }
    public boolean hasMultipleTransformations() { return false; }
    public String toString() { return url; }
}
