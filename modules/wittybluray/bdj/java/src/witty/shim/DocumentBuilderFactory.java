package witty.shim;

import javax.xml.parsers.ParserConfigurationException;

/** javax.xml.parsers.DocumentBuilderFactory for disc code: see Xml. */
public class DocumentBuilderFactory {
    private boolean namespaces, ignoreSpace, ignoreComments, coalesce;
    protected DocumentBuilderFactory() {}
    public static DocumentBuilderFactory newInstance() { return new DocumentBuilderFactory(); }
    public void setNamespaceAware(boolean b) { namespaces = b; }
    public boolean isNamespaceAware() { return namespaces; }
    public void setValidating(boolean b) {}
    public boolean isValidating() { return false; }
    public void setIgnoringElementContentWhitespace(boolean b) { ignoreSpace = b; }
    public boolean isIgnoringElementContentWhitespace() { return ignoreSpace; }
    public void setExpandEntityReferences(boolean b) {}
    public boolean isExpandEntityReferences() { return true; }
    public void setIgnoringComments(boolean b) { ignoreComments = b; }
    public boolean isIgnoringComments() { return ignoreComments; }
    public void setCoalescing(boolean b) { coalesce = b; }
    public boolean isCoalescing() { return coalesce; }
    public void setAttribute(String n, Object v) {}
    public Object getAttribute(String n) { return null; }
    public void setFeature(String n, boolean v) {}
    public boolean getFeature(String n) { return false; }
    public void setXIncludeAware(boolean b) {}
    public boolean isXIncludeAware() { return false; }
    public DocumentBuilder newDocumentBuilder() throws ParserConfigurationException {
        javax.xml.parsers.DocumentBuilderFactory f = javax.xml.parsers.DocumentBuilderFactory.newInstance();
        f.setNamespaceAware(namespaces);
        f.setIgnoringElementContentWhitespace(ignoreSpace);
        f.setIgnoringComments(ignoreComments);
        f.setCoalescing(coalesce);
        for (int i = 0; i < Xml.FEATURES.length; i++) {
            try {
                f.setFeature(Xml.FEATURES[i][0], Boolean.parseBoolean(Xml.FEATURES[i][1]));
            } catch (ParserConfigurationException e) {
                // A parser without the feature does not have what it would turn off.
            }
        }
        f.setXIncludeAware(false);
        return new DocumentBuilder(f.newDocumentBuilder());
    }
}
