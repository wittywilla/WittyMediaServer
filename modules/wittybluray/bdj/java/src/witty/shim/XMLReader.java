package witty.shim;

import java.io.IOException;
import org.xml.sax.ContentHandler;
import org.xml.sax.DTDHandler;
import org.xml.sax.EntityResolver;
import org.xml.sax.ErrorHandler;
import org.xml.sax.InputSource;
import org.xml.sax.SAXException;
import org.xml.sax.SAXNotRecognizedException;
import org.xml.sax.SAXNotSupportedException;

/** org.xml.sax.XMLReader for disc code: see Xml. */
public interface XMLReader {
    boolean getFeature(String n) throws SAXNotRecognizedException, SAXNotSupportedException;
    void setFeature(String n, boolean v) throws SAXNotRecognizedException, SAXNotSupportedException;
    Object getProperty(String n) throws SAXNotRecognizedException, SAXNotSupportedException;
    void setProperty(String n, Object v) throws SAXNotRecognizedException, SAXNotSupportedException;
    void setEntityResolver(EntityResolver r);
    EntityResolver getEntityResolver();
    void setDTDHandler(DTDHandler h);
    DTDHandler getDTDHandler();
    void setContentHandler(ContentHandler h);
    ContentHandler getContentHandler();
    void setErrorHandler(ErrorHandler h);
    ErrorHandler getErrorHandler();
    void parse(InputSource s) throws IOException, SAXException;
    void parse(String name) throws IOException, SAXException;

    /** The parser's own reader, held at arm's length. */
    final class Checked implements XMLReader {
        private final org.xml.sax.XMLReader real;
        private EntityResolver own;
        Checked(org.xml.sax.XMLReader r) {
            real = r;
            Xml.arm(r);
        }
        public boolean getFeature(String n) throws SAXNotRecognizedException, SAXNotSupportedException { return real.getFeature(n); }
        public void setFeature(String n, boolean v) throws SAXNotRecognizedException, SAXNotSupportedException {
            if (Xml.harmless(n)) real.setFeature(n, v);
        }
        public Object getProperty(String n) throws SAXNotRecognizedException, SAXNotSupportedException { return real.getProperty(n); }
        public void setProperty(String n, Object v) throws SAXNotRecognizedException, SAXNotSupportedException {
            // The two that only add callbacks; the others set what a parser may fetch.
            if ("http://xml.org/sax/properties/lexical-handler".equals(n) || "http://xml.org/sax/properties/declaration-handler".equals(n)) real.setProperty(n, v);
        }
        public void setEntityResolver(EntityResolver r) {
            own = r;
            real.setEntityResolver(Xml.checked(r));
        }
        public EntityResolver getEntityResolver() { return own; }
        public void setDTDHandler(DTDHandler h) { real.setDTDHandler(h); }
        public DTDHandler getDTDHandler() { return real.getDTDHandler(); }
        public void setContentHandler(ContentHandler h) { real.setContentHandler(h); }
        public ContentHandler getContentHandler() { return real.getContentHandler(); }
        public void setErrorHandler(ErrorHandler h) { real.setErrorHandler(h); }
        public ErrorHandler getErrorHandler() { return real.getErrorHandler(); }
        public void parse(InputSource s) throws IOException, SAXException { real.parse(Xml.open(s)); }
        public void parse(String name) throws IOException, SAXException { real.parse(new InputSource(Xml.open(name))); }
    }
}
