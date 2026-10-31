package witty.shim;

import javax.xml.parsers.ParserConfigurationException;
import org.xml.sax.SAXException;

/** javax.xml.parsers.SAXParserFactory for disc code: see Xml. */
public class SAXParserFactory {
    private boolean namespaces;
    protected SAXParserFactory() {}
    public static SAXParserFactory newInstance() { return new SAXParserFactory(); }
    public void setNamespaceAware(boolean b) { namespaces = b; }
    public boolean isNamespaceAware() { return namespaces; }
    public void setValidating(boolean b) {}
    public boolean isValidating() { return false; }
    public void setFeature(String n, boolean v) {}
    public boolean getFeature(String n) { return false; }
    public void setXIncludeAware(boolean b) {}
    public boolean isXIncludeAware() { return false; }
    public SAXParser newSAXParser() throws ParserConfigurationException, SAXException {
        javax.xml.parsers.SAXParserFactory f = javax.xml.parsers.SAXParserFactory.newInstance();
        f.setNamespaceAware(namespaces);
        f.setValidating(false);
        f.setXIncludeAware(false);
        for (int i = 0; i < Xml.FEATURES.length; i++) {
            try {
                f.setFeature(Xml.FEATURES[i][0], Boolean.parseBoolean(Xml.FEATURES[i][1]));
            } catch (SAXException e) {
                // A parser without the feature does not have what it would turn off.
            }
        }
        return new SAXParser(f.newSAXParser());
    }
}
