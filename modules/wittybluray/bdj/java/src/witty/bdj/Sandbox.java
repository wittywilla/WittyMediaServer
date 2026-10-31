package witty.bdj;

import java.util.HashMap;
import java.util.HashSet;
import java.util.Map;
import java.util.Set;

/**
 * What a disc's code may touch. A disc is untrusted input, and BD-J is code: so
 * its classes load through DiscLoader, which consults this default-deny list.
 * A class that is not listed cannot be linked by disc code at all, whatever
 * tricks it tries, because the JVM asks the defining loader to resolve every
 * name. Reflection, class loading, processes, native code, sockets and raw file
 * access are not listed; the ones discs legitimately need are renamed to shims
 * that check what they are asked to do.
 */
final class Sandbox {
    private Sandbox() {}

    /** JDK classes replaced in a disc's class files by their checked versions. */
    static final Map<String, String> RENAME = new HashMap<String, String>();
    static {
        String[][] r = {
            {"java/io/File", "File"}, {"java/io/FileInputStream", "FileInputStream"}, {"java/io/FileOutputStream", "FileOutputStream"},
            {"java/io/FileReader", "FileReader"}, {"java/io/FileWriter", "FileWriter"}, {"java/io/RandomAccessFile", "RandomAccessFile"},
            {"java/io/FilenameFilter", "FilenameFilter"}, {"java/io/FileFilter", "FileFilter"},
            {"java/io/PrintStream", "PrintStream"}, {"java/io/PrintWriter", "PrintWriter"},
            {"java/lang/System", "System"}, {"java/lang/Runtime", "Runtime"}, {"java/lang/ClassLoader", "Loader"},
            {"java/net/URL", "URL"}, {"java/net/Socket", "Socket"}, {"java/net/InetAddress", "InetAddress"},
            {"java/awt/Toolkit", "Toolkit"},
            {"javax/xml/parsers/DocumentBuilderFactory", "DocumentBuilderFactory"}, {"javax/xml/parsers/DocumentBuilder", "DocumentBuilder"},
            {"javax/xml/parsers/SAXParserFactory", "SAXParserFactory"}, {"javax/xml/parsers/SAXParser", "SAXParser"},
            {"org/xml/sax/XMLReader", "XMLReader"},
        };
        for (int i = 0; i < r.length; i++) RENAME.put(r[i][0], "witty/shim/" + r[i][1]);
    }

    private static final Set<String> EXACT = new HashSet<String>();
    /**
     * Whole packages, only where every class in them is harmless. Before adding one,
     * read all its classes: java.rmi opens sockets, the XML packages fetch what a
     * document names, and java.awt.color.ICC_Profile reads and writes any file.
     */
    private static final String[] PACKAGES = {
        "java.lang.ref.", "java.math.", "java.text.", "java.util.concurrent.", "java.awt.event.", "java.awt.image.", "java.awt.geom.",
        "java.awt.font.", "org.w3c.dom.traversal.", "org.w3c.dom.events.", "org.w3c.dom.ranges.",
    };
    static {
        String[] xml = {"java.awt.color.ColorSpace", "java.awt.color.CMMException", "java.awt.color.ProfileDataException",
            "javax.xml.parsers.ParserConfigurationException", "javax.xml.parsers.FactoryConfigurationError",
            "org.w3c.dom.Attr", "org.w3c.dom.CDATASection", "org.w3c.dom.CharacterData", "org.w3c.dom.Comment", "org.w3c.dom.DOMConfiguration", "org.w3c.dom.DOMError", "org.w3c.dom.DOMErrorHandler", "org.w3c.dom.DOMException",
            "org.w3c.dom.DOMImplementation", "org.w3c.dom.DOMLocator", "org.w3c.dom.DOMStringList", "org.w3c.dom.Document", "org.w3c.dom.DocumentFragment", "org.w3c.dom.DocumentType", "org.w3c.dom.Element", "org.w3c.dom.Entity",
            "org.w3c.dom.EntityReference", "org.w3c.dom.NameList", "org.w3c.dom.NamedNodeMap", "org.w3c.dom.Node", "org.w3c.dom.NodeList", "org.w3c.dom.Notation", "org.w3c.dom.ProcessingInstruction", "org.w3c.dom.Text", "org.w3c.dom.TypeInfo", "org.w3c.dom.UserDataHandler",
            "org.xml.sax.AttributeList", "org.xml.sax.Attributes", "org.xml.sax.ContentHandler", "org.xml.sax.DTDHandler", "org.xml.sax.DocumentHandler", "org.xml.sax.EntityResolver", "org.xml.sax.ErrorHandler", "org.xml.sax.HandlerBase",
            "org.xml.sax.InputSource", "org.xml.sax.Locator", "org.xml.sax.SAXException", "org.xml.sax.SAXNotRecognizedException", "org.xml.sax.SAXNotSupportedException", "org.xml.sax.SAXParseException",
            "org.xml.sax.helpers.AttributeListImpl", "org.xml.sax.helpers.AttributesImpl", "org.xml.sax.helpers.DefaultHandler", "org.xml.sax.helpers.LocatorImpl", "org.xml.sax.helpers.NamespaceSupport",
            "org.xml.sax.ext.Attributes2", "org.xml.sax.ext.Attributes2Impl", "org.xml.sax.ext.DeclHandler", "org.xml.sax.ext.DefaultHandler2", "org.xml.sax.ext.EntityResolver2", "org.xml.sax.ext.LexicalHandler", "org.xml.sax.ext.Locator2", "org.xml.sax.ext.Locator2Impl",
            // Inter-application communication names these and needs no more of RMI.
            "java.rmi.Remote", "java.rmi.RemoteException", "java.rmi.NotBoundException", "java.rmi.AlreadyBoundException", "java.rmi.AccessException", "java.rmi.ServerException", "java.rmi.ServerError", "java.rmi.NoSuchObjectException", "java.rmi.MarshalException", "java.rmi.UnmarshalException", "java.rmi.UnexpectedException"};
        for (int i = 0; i < xml.length; i++) EXACT.add(xml[i]);
        String[] lang = {"Object", "String", "StringBuffer", "StringBuilder", "CharSequence", "Comparable", "Runnable", "Cloneable", "Iterable", "AutoCloseable",
            "Math", "StrictMath", "Number", "Integer", "Long", "Short", "Byte", "Character", "Boolean", "Float", "Double", "Void", "Enum",
            "Thread", "Thread$State", "Thread$UncaughtExceptionHandler", "ThreadGroup", "ThreadLocal", "InheritableThreadLocal", "Throwable", "Class", "StackTraceElement",
            "Deprecated", "Override", "SuppressWarnings", "SecurityManager", "Character$Subset", "Character$UnicodeBlock"};
        for (int i = 0; i < lang.length; i++) EXACT.add("java.lang." + lang[i]);
        String[] io = {"InputStream", "OutputStream", "ByteArrayInputStream", "ByteArrayOutputStream", "BufferedInputStream", "BufferedOutputStream",
            "DataInputStream", "DataOutputStream", "DataInput", "DataOutput", "FilterInputStream", "FilterOutputStream", "PushbackInputStream", "SequenceInputStream",
            "Reader", "Writer", "BufferedReader", "BufferedWriter", "InputStreamReader", "OutputStreamWriter", "StringReader", "StringWriter", "CharArrayReader", "CharArrayWriter",
            "FilterReader", "FilterWriter", "PushbackReader", "LineNumberReader", "StreamTokenizer", "Serializable", "Closeable", "Flushable", "Externalizable",
            "IOException", "EOFException", "FileNotFoundException", "InterruptedIOException", "UnsupportedEncodingException", "UTFDataFormatException", "CharConversionException", "NotSerializableException"};
        for (int i = 0; i < io.length; i++) EXACT.add("java.io." + io[i]);
        String[] util = {"AbstractCollection", "AbstractList", "AbstractMap", "AbstractSet", "AbstractSequentialList", "AbstractQueue", "ArrayList", "ArrayDeque", "Arrays", "BitSet", "Calendar", "Collection", "Collections",
            "Comparator", "ConcurrentModificationException", "Currency", "Date", "Deque", "Dictionary", "EmptyStackException", "Enumeration", "EventListener", "EventListenerProxy", "EventObject",
            "GregorianCalendar", "HashMap", "HashSet", "Hashtable", "IdentityHashMap", "Iterator", "LinkedHashMap", "LinkedHashSet", "LinkedList", "List", "ListIterator", "ListResourceBundle",
            "Locale", "Map", "Map$Entry", "MissingResourceException", "NavigableMap", "NavigableSet", "NoSuchElementException", "Observable", "Observer", "PriorityQueue", "Properties", "PropertyResourceBundle",
            "Queue", "Random", "RandomAccess", "ResourceBundle", "Set", "SimpleTimeZone", "SortedMap", "SortedSet", "Stack", "StringTokenizer", "TimeZone", "Timer", "TimerTask", "TooManyListenersException",
            "TreeMap", "TreeSet", "UUID", "Vector", "WeakHashMap", "zip.Adler32", "zip.CRC32", "zip.Checksum", "zip.CheckedInputStream", "zip.CheckedOutputStream", "zip.DataFormatException", "zip.Deflater", "zip.DeflaterOutputStream",
            "zip.GZIPInputStream", "zip.GZIPOutputStream", "zip.Inflater", "zip.InflaterInputStream", "zip.ZipEntry", "zip.ZipException", "zip.ZipInputStream", "zip.ZipOutputStream"};
        for (int i = 0; i < util.length; i++) EXACT.add("java.util." + util[i]);
        String[] sec = {"java.security.AccessController", "java.security.PrivilegedAction", "java.security.PrivilegedExceptionAction", "java.security.PrivilegedActionException",
            "java.security.AccessControlException", "java.security.Permission", "java.security.BasicPermission", "java.security.MessageDigest", "java.security.DigestException",
            "java.security.NoSuchAlgorithmException", "java.security.GeneralSecurityException", "java.security.SecureRandom", "java.security.Principal", "java.security.cert.Certificate",
            "java.security.cert.CertificateException", "java.security.cert.X509Certificate", "javax.security.auth.x500.X500Principal",
            "java.net.URLEncoder", "java.net.URLDecoder", "java.net.URI", "java.net.MalformedURLException", "java.net.URISyntaxException", "java.net.SocketException", "java.net.UnknownHostException", "java.net.ConnectException",
            "java.lang.reflect.Array", "java.lang.reflect.InvocationTargetException",
            "java.nio.ByteBuffer", "java.nio.Buffer", "java.nio.ByteOrder", "java.nio.CharBuffer", "java.nio.IntBuffer", "java.nio.BufferUnderflowException", "java.nio.BufferOverflowException", "java.nio.charset.Charset"};
        for (int i = 0; i < sec.length; i++) EXACT.add(sec[i]);
        String[] awt = {"Adjustable", "AlphaComposite", "AWTError", "AWTEvent", "AWTEventMulticaster", "AWTException", "BasicStroke", "BorderLayout", "CardLayout", "Color", "Component", "Component$BaselineResizeBehavior", "Composite", "CompositeContext",
            "Container", "Cursor", "Dimension", "EventQueue", "FlowLayout", "Font", "FontFormatException", "FontMetrics", "GradientPaint", "Graphics", "Graphics2D", "GraphicsConfiguration", "GraphicsDevice", "GraphicsEnvironment",
            "GridBagConstraints", "GridBagLayout", "GridLayout", "HeadlessException", "IllegalComponentStateException", "Image", "Insets", "ItemSelectable", "KeyboardFocusManager", "KeyEventDispatcher", "KeyEventPostProcessor", "LayoutManager", "LayoutManager2",
            "LinearGradientPaint", "MediaTracker", "MultipleGradientPaint", "Paint", "PaintContext", "Point", "Polygon", "RadialGradientPaint", "Rectangle", "RenderingHints", "RenderingHints$Key", "Shape", "Stroke", "TexturePaint", "Transparency", "Frame", "Window"};
        for (int i = 0; i < awt.length; i++) EXACT.add("java.awt." + awt[i]);
    }

    /** The BD-J platform this runtime provides. */
    static boolean isPlatform(String name) {
        return name.startsWith("org.havi.") || name.startsWith("org.dvb.") || name.startsWith("org.davic.") || name.startsWith("org.bluray.")
            || name.startsWith("javax.tv.") || name.startsWith("javax.media.") || name.startsWith("javax.microedition.") || name.startsWith("witty.shim.");
    }

    /** A name a disc must not define itself. */
    static boolean isReserved(String name) {
        return name.startsWith("java.") || name.startsWith("javax.") || name.startsWith("sun.") || name.startsWith("jdk.") || name.startsWith("com.sun.") || name.startsWith("witty.");
    }

    static boolean allowedJdk(String name) {
        if (EXACT.contains(name)) return true;
        if (name.startsWith("java.lang.") && name.indexOf('.', 10) < 0 && (name.endsWith("Exception") || name.endsWith("Error"))) return true;
        for (int i = 0; i < PACKAGES.length; i++) if (name.startsWith(PACKAGES[i])) return true;
        return false;
    }
}
