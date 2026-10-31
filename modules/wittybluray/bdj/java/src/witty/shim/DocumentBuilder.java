package witty.shim;

import java.io.IOException;
import java.io.InputStream;
import org.w3c.dom.DOMImplementation;
import org.w3c.dom.Document;
import org.xml.sax.EntityResolver;
import org.xml.sax.ErrorHandler;
import org.xml.sax.InputSource;
import org.xml.sax.SAXException;

/** javax.xml.parsers.DocumentBuilder for disc code: see Xml. */
public class DocumentBuilder {
    private final javax.xml.parsers.DocumentBuilder real;
    DocumentBuilder(javax.xml.parsers.DocumentBuilder r) {
        real = r;
        real.setEntityResolver(Xml.NOTHING);
    }
    public Document parse(InputStream in) throws SAXException, IOException { return real.parse(Xml.open(new InputSource(in))); }
    public Document parse(InputStream in, String systemId) throws SAXException, IOException { return parse(in); }
    public Document parse(String name) throws SAXException, IOException { return real.parse(new InputSource(Xml.open(name))); }
    public Document parse(File f) throws SAXException, IOException { return real.parse(new InputSource(new FileInputStream(f))); }
    public Document parse(InputSource s) throws SAXException, IOException { return real.parse(Xml.open(s)); }
    public boolean isNamespaceAware() { return real.isNamespaceAware(); }
    public boolean isValidating() { return false; }
    public void setEntityResolver(EntityResolver r) { real.setEntityResolver(Xml.checked(r)); }
    public void setErrorHandler(ErrorHandler h) { real.setErrorHandler(h); }
    public Document newDocument() { return real.newDocument(); }
    public DOMImplementation getDOMImplementation() { return real.getDOMImplementation(); }
    public void reset() { real.reset(); real.setEntityResolver(Xml.NOTHING); }
}
