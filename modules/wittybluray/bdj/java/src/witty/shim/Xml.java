package witty.shim;

import java.io.ByteArrayInputStream;
import java.io.IOException;
import org.xml.sax.EntityResolver;
import org.xml.sax.InputSource;
import org.xml.sax.SAXException;
import org.xml.sax.XMLReader;

/**
 * What the XML shims share. A parser left to itself fetches whatever a document
 * or a system identifier names, by any protocol, which would hand disc code the
 * whole machine. So a parser here reads only the bytes it is given: names go
 * through URL, external entities and DTDs read as empty, and the settings that
 * would change that are not passed on.
 */
final class Xml {
    private Xml() {}

    /** Set on every factory; the two later ones are for parsers that know them. */
    static final String[][] FEATURES = {
        {"http://javax.xml.XMLConstants/feature/secure-processing", "true"},
        {"http://xml.org/sax/features/external-general-entities", "false"},
        {"http://xml.org/sax/features/external-parameter-entities", "false"},
        {"http://apache.org/xml/features/nonvalidating/load-external-dtd", "false"},
        {"http://apache.org/xml/features/xinclude", "false"},
    };

    /** Applications may ask for these and nothing else. */
    static boolean harmless(String feature) {
        return feature != null && (feature.equals("http://xml.org/sax/features/namespaces") || feature.equals("http://xml.org/sax/features/namespace-prefixes")
            || feature.equals("http://xml.org/sax/features/string-interning") || feature.equals("http://xml.org/sax/features/xmlns-uris"));
    }

    /** A source that holds bytes or characters, with no name a parser could follow. */
    static InputSource open(InputSource s) throws IOException {
        if (s == null) throw new IllegalArgumentException("no input");
        InputSource o = new InputSource();
        o.setEncoding(s.getEncoding());
        if (s.getCharacterStream() != null) o.setCharacterStream(s.getCharacterStream());
        else if (s.getByteStream() != null) o.setByteStream(s.getByteStream());
        else o.setByteStream(open(s.getSystemId()));
        return o;
    }

    /** What a system identifier names, as the disc's own files allow it. */
    static java.io.InputStream open(String name) throws IOException {
        if (name == null) throw new IllegalArgumentException("no input");
        if (name.indexOf(':') > 1) return new URL(name).openStream();
        return new FileInputStream(name);
    }

    static final EntityResolver NOTHING = new EntityResolver() {
        public InputSource resolveEntity(String publicId, String systemId) { return empty(); }
    };

    static InputSource empty() { return new InputSource(new ByteArrayInputStream(new byte[0])); }

    /** Lets an application's own resolver answer with bytes it holds; a name it answers with is checked too. */
    static EntityResolver checked(final EntityResolver r) {
        if (r == null) return NOTHING;
        return new EntityResolver() {
            public InputSource resolveEntity(String publicId, String systemId) throws SAXException, IOException {
                InputSource s = r.resolveEntity(publicId, systemId);
                return s == null ? empty() : open(s);
            }
        };
    }

    static void arm(XMLReader r) {
        for (int i = 0; i < FEATURES.length; i++) {
            try {
                r.setFeature(FEATURES[i][0], Boolean.parseBoolean(FEATURES[i][1]));
            } catch (SAXException e) {
                // A parser without the feature does not have what it would turn off.
            }
        }
        r.setEntityResolver(NOTHING);
    }
}
