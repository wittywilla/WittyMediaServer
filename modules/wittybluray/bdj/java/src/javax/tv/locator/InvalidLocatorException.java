package javax.tv.locator;
public class InvalidLocatorException extends Exception {
    private transient Locator loc;
    public InvalidLocatorException() {}
    public InvalidLocatorException(String m) { super(m); }
    public InvalidLocatorException(Locator l) { loc = l; }
    public InvalidLocatorException(Locator l, String m) { super(m); loc = l; }
    public Locator getInvalidLocator() { return loc; }
}
