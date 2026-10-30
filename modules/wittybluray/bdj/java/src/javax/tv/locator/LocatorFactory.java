package javax.tv.locator;
public abstract class LocatorFactory {
    private static final LocatorFactory it = new LocatorFactory() {};
    public static LocatorFactory getInstance() { return it; }
    public Locator createLocator(String s) throws MalformedLocatorException {
        try {
            return new org.bluray.net.BDLocator(s);
        } catch (org.davic.net.InvalidLocatorException e) {
            throw new MalformedLocatorException(s);
        }
    }
    public Locator[] transformLocator(Locator l) throws InvalidLocatorException { return new Locator[] {l}; }
}
