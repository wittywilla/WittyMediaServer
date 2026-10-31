package witty.shim;

import java.io.IOException;
import java.io.InputStream;
import org.xml.sax.HandlerBase;
import org.xml.sax.InputSource;
import org.xml.sax.SAXException;
import org.xml.sax.helpers.DefaultHandler;

/** javax.xml.parsers.SAXParser for disc code: see Xml. */
public class SAXParser {
    private final javax.xml.parsers.SAXParser real;
    private final XMLReader reader;
    SAXParser(javax.xml.parsers.SAXParser r) throws SAXException {
        real = r;
        reader = new XMLReader.Checked(r.getXMLReader());
    }
    public void parse(InputSource s, DefaultHandler h) throws SAXException, IOException {
        reader.setContentHandler(h);
        reader.setDTDHandler(h);
        reader.setErrorHandler(h);
        reader.parse(s);
    }
    public void parse(InputStream in, DefaultHandler h) throws SAXException, IOException { parse(new InputSource(in), h); }
    public void parse(InputStream in, DefaultHandler h, String systemId) throws SAXException, IOException { parse(new InputSource(in), h); }
    public void parse(String name, DefaultHandler h) throws SAXException, IOException { parse(new InputSource(Xml.open(name)), h); }
    public void parse(File f, DefaultHandler h) throws SAXException, IOException { parse(new InputSource(new FileInputStream(f)), h); }
    public XMLReader getXMLReader() { return reader; }
    public boolean isNamespaceAware() { return real.isNamespaceAware(); }
    public boolean isValidating() { return false; }
    public void setProperty(String n, Object v) throws org.xml.sax.SAXNotRecognizedException, org.xml.sax.SAXNotSupportedException { reader.setProperty(n, v); }
    public Object getProperty(String n) throws org.xml.sax.SAXNotRecognizedException, org.xml.sax.SAXNotSupportedException { return reader.getProperty(n); }
}
