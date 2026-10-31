package a;

import java.awt.Color;
import java.awt.Component;
import java.awt.Graphics;
import javax.tv.xlet.Xlet;
import javax.tv.xlet.XletContext;
import org.bluray.system.RegisterAccess;
import org.havi.ui.HScene;
import org.havi.ui.HSceneFactory;

/**
 * A hostile disc. Each probe tries one way out of the sandbox and reports through
 * a GPR: 1 = it failed as it should, 2 = it got through. The Go test also looks
 * for the probes' effects from outside (a file appearing, a socket connecting), so
 * nothing rests on this class telling the truth. Lambdas are avoided on purpose:
 * disc code cannot link what they compile to.
 *
 * The parameter is the mode. For "probes" the disc's storage holds args.txt: a
 * secret XML file, a path to write, a forbidden local port, a free port, a folder
 * that is off limits, and the secret file's URI, one to a line.
 */
public class AttackXlet implements Xlet {
    public static final int FIRST = 200;
    /** Sets GPR 199 once every probe has run. */
    public static final int DONE = 199;
    private String[] args;
    private int next = FIRST;
    private XletContext ctx;

    private interface Probe {
        /** true: it got through. A throw of any kind means it did not. */
        boolean run() throws Throwable;
    }

    private static void report(int gpr, int v) { RegisterAccess.getInstance().setGPR(gpr, v); }

    /** Probes run in a fixed order, so the Go test can name each by its register. */
    private void p(String name, Probe x) {
        int gpr = next++;
        boolean through = false;
        String why = "refused";
        try {
            through = x.run();
        } catch (Throwable t) {
            why = t.getClass().getName() + ": " + t.getMessage();
        }
        System.out.println("probe " + gpr + " " + name + ": " + (through ? "GOT THROUGH" : why));
        report(gpr, through ? 2 : 1);
    }

    public void initXlet(XletContext c) {
        ctx = c;
        args = (String[]) c.getXletProperty(XletContext.ARGS);
    }

    public void startXlet() {
        String mode = args != null && args.length > 0 ? args[0] : "probes";
        if (mode.equals("probes")) {
            probes();
            report(DONE, 1);
        } else if (mode.equals("exit")) {
            draw();
            ctx.notifyDestroyed();
        } else if (mode.equals("hang-init")) {
            spin();
        } else {
            draw();
            report(DONE, 1);
            Hostile.go(mode);
        }
    }

    private void draw() {
        HScene s = HSceneFactory.getInstance().getDefaultHScene();
        Component box = new Component() {
            public void paint(Graphics g) {
                g.setColor(Color.red);
                g.fillRect(0, 0, 100, 100);
            }
        };
        box.setBounds(0, 0, 100, 100);
        s.add(box);
        s.setVisible(true);
        s.repaint();
    }

    private static void spin() {
        for (;;) {
            if (Thread.interrupted()) Thread.yield();
        }
    }

    public void pauseXlet() {}
    public void destroyXlet(boolean u) {}

    /** The test's paths are too long for a BD-J object's parameters: they come in a file in the disc's storage. */
    private static String[] given() {
        try {
            java.io.BufferedReader r = new java.io.BufferedReader(new java.io.InputStreamReader(new java.io.FileInputStream(new java.io.File(System.getProperty("dvb.persistent.root"), "args.txt")), "UTF-8"));
            String[] a = new String[6];
            for (int i = 0; i < a.length; i++) a[i] = r.readLine();
            r.close();
            return a;
        } catch (java.io.IOException e) {
            throw new IllegalStateException(e.toString());
        }
    }

    private void probes() {
        String[] g = given();
        final String secret = g[0], target = g[1], port = g[2], free = g[3], offLimits = g[4], secretUri = g[5];
        final String local = "http://127.0.0.1:" + port + "/x";
        // --- The network, through what the allow list lets through by accident.
        p("rmi connect", new Probe() { public boolean run() throws Throwable {
            java.rmi.Naming.list("rmi://127.0.0.1:" + port + "/");
            return true;
        }});
        p("rmi registry listens", new Probe() { public boolean run() throws Throwable {
            java.rmi.registry.LocateRegistry.createRegistry(Integer.parseInt(free));
            return true;
        }});
        p("rmi get registry", new Probe() { public boolean run() throws Throwable {
            java.rmi.registry.LocateRegistry.getRegistry("127.0.0.1", Integer.parseInt(port)).list();
            return true;
        }});
        p("socket shim", new Probe() { public boolean run() throws Throwable {
            new java.net.Socket("127.0.0.1", Integer.parseInt(port));
            return true;
        }});
        p("server socket", new Probe() { public boolean run() throws Throwable { Blocked.serverSocket(free); return true; }});
        p("datagram", new Probe() { public boolean run() throws Throwable { Blocked.datagram(port); return true; }});
        p("channel", new Probe() { public boolean run() throws Throwable { Blocked.channel(port); return true; }});
        p("url http", new Probe() { public boolean run() throws Throwable {
            new java.net.URL(local).openStream();
            return true;
        }});
        p("url connection", new Probe() { public boolean run() throws Throwable { Blocked.urlConnection(local); return true; }});
        p("url from uri", new Probe() { public boolean run() throws Throwable {
            java.net.URI.create(local).toURL().openStream();
            return true;
        }});
        p("url jar", new Probe() { public boolean run() throws Throwable {
            new java.net.URL("jar:" + secretUri + "!/x").openStream();
            return true;
        }});
        p("media locator", new Probe() { public boolean run() throws Throwable {
            javax.media.Manager.createPlayer(new javax.media.MediaLocator(local)).start();
            Thread.sleep(200);
            return false; // the Go test sees whether anything connected
        }});
        // --- Files, through XML.
        p("dom reads a file", new Probe() { public boolean run() throws Throwable {
            return textOf(javax.xml.parsers.DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(secretUri)).indexOf("TOPSECRET") >= 0;
        }});
        p("dom fetches a url", new Probe() { public boolean run() throws Throwable {
            javax.xml.parsers.DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(local);
            return true;
        }});
        p("dom input source", new Probe() { public boolean run() throws Throwable {
            return textOf(javax.xml.parsers.DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(new org.xml.sax.InputSource(secretUri))).indexOf("TOPSECRET") >= 0;
        }});
        final String xxe = "<!DOCTYPE s [<!ENTITY e SYSTEM \"" + secretUri + "\">]><s>&e;</s>";
        p("dom external entity", new Probe() { public boolean run() throws Throwable {
            javax.xml.parsers.DocumentBuilderFactory f = javax.xml.parsers.DocumentBuilderFactory.newInstance();
            f.setAttribute("http://javax.xml.XMLConstants/property/accessExternalDTD", "all");
            f.setAttribute("http://javax.xml.XMLConstants/property/accessExternalSchema", "all");
            f.setFeature("http://javax.xml.XMLConstants/feature/secure-processing", false);
            return textOf(f.newDocumentBuilder().parse(new java.io.ByteArrayInputStream(xxe.getBytes("UTF-8")))).indexOf("TOPSECRET") >= 0;
        }});
        p("dom xinclude", new Probe() { public boolean run() throws Throwable {
            javax.xml.parsers.DocumentBuilderFactory f = javax.xml.parsers.DocumentBuilderFactory.newInstance();
            f.setNamespaceAware(true);
            f.setXIncludeAware(true);
            String doc = "<a xmlns:xi=\"http://www.w3.org/2001/XInclude\"><xi:include href=\"" + secretUri + "\"/></a>";
            return textOf(f.newDocumentBuilder().parse(new java.io.ByteArrayInputStream(doc.getBytes("UTF-8")))).indexOf("TOPSECRET") >= 0;
        }});
        p("sax reads a file", new Probe() { public boolean run() throws Throwable {
            Grab g = new Grab();
            javax.xml.parsers.SAXParserFactory.newInstance().newSAXParser().parse(secretUri, g);
            return g.text.indexOf("TOPSECRET") >= 0;
        }});
        p("sax external entity", new Probe() { public boolean run() throws Throwable {
            Grab g = new Grab();
            javax.xml.parsers.SAXParserFactory f = javax.xml.parsers.SAXParserFactory.newInstance();
            f.setFeature("http://javax.xml.XMLConstants/feature/secure-processing", false);
            f.newSAXParser().parse(new java.io.ByteArrayInputStream(xxe.getBytes("UTF-8")), g);
            return g.text.indexOf("TOPSECRET") >= 0;
        }});
        p("sax reader", new Probe() { public boolean run() throws Throwable {
            Grab g = new Grab();
            org.xml.sax.XMLReader r = javax.xml.parsers.SAXParserFactory.newInstance().newSAXParser().getXMLReader();
            r.setContentHandler(g);
            r.parse(secretUri);
            return g.text.indexOf("TOPSECRET") >= 0;
        }});
        p("sax reader factory", new Probe() { public boolean run() throws Throwable {
            Grab g = new Grab();
            org.xml.sax.XMLReader r = org.xml.sax.helpers.XMLReaderFactory.createXMLReader();
            r.setContentHandler(g);
            r.parse(secretUri);
            return g.text.indexOf("TOPSECRET") >= 0;
        }});
        p("sax parser factory makes any class", new Probe() { public boolean run() throws Throwable {
            try {
                org.xml.sax.helpers.ParserFactory.makeParser("java.lang.Thread");
            } catch (ClassCastException e) {
                return true; // it built the object
            }
            return true;
        }});
        p("dom load and save", new Probe() { public boolean run() throws Throwable {
            org.w3c.dom.Document d = javax.xml.parsers.DocumentBuilderFactory.newInstance().newDocumentBuilder().newDocument();
            org.w3c.dom.ls.DOMImplementationLS ls = (org.w3c.dom.ls.DOMImplementationLS) d.getImplementation().getFeature("LS", "3.0");
            return textOf(ls.createLSParser(org.w3c.dom.ls.DOMImplementationLS.MODE_SYNCHRONOUS, null).parseURI(secretUri)).indexOf("TOPSECRET") >= 0;
        }});
        p("properties xml", new Probe() { public boolean run() throws Throwable {
            java.util.Properties x = new java.util.Properties();
            String doc = "<!DOCTYPE properties SYSTEM \"" + secretUri + "\"><properties version=\"1.0\"><entry key=\"a\">b</entry></properties>";
            x.loadFromXML(new java.io.ByteArrayInputStream(doc.getBytes("UTF-8")));
            return false; // the entity cannot be read back this way; only a crash would matter
        }});
        // --- Files, through colour profiles.
        p("icc profile writes any file", new Probe() { public boolean run() throws Throwable {
            java.awt.color.ICC_Profile.getInstance(java.awt.color.ColorSpace.CS_sRGB).write(target);
            return new java.io.File(target).length() >= 0;
        }});
        p("icc profile reads any file", new Probe() { public boolean run() throws Throwable {
            try {
                java.awt.color.ICC_Profile.getInstance(secret);
            } catch (IllegalArgumentException e) {
                return true; // it opened the file and judged its contents
            }
            return true;
        }});
        // --- Files, directly.
        p("read outside", new Probe() { public boolean run() throws Throwable {
            new java.io.FileInputStream(secret);
            return true;
        }});
        p("write outside", new Probe() { public boolean run() throws Throwable {
            new java.io.FileOutputStream(target + ".2");
            return true;
        }});
        p("random access outside", new Probe() { public boolean run() throws Throwable {
            new java.io.RandomAccessFile(secret, "r");
            return true;
        }});
        p("relative climb", new Probe() { public boolean run() throws Throwable {
            String vfs = System.getProperty("bluray.vfs.root");
            StringBuffer up = new StringBuffer();
            for (int i = 0; i < 40; i++) up.append("../");
            new java.io.FileInputStream(up + secret.replace(':', '/'));
            new java.io.FileInputStream(vfs + "/" + up + secret.replace(':', '/'));
            return true;
        }});
        p("list outside", new Probe() { public boolean run() throws Throwable { return new java.io.File(offLimits).list() != null; }});
        p("exists outside", new Probe() { public boolean run() throws Throwable { return new java.io.File(secret).exists() || new java.io.File(secret).length() > 0; }});
        p("rename out", new Probe() { public boolean run() throws Throwable {
            java.io.File mine = new java.io.File(System.getProperty("dvb.persistent.root"), "x.bin");
            new java.io.FileOutputStream(mine).close();
            return mine.renameTo(new java.io.File(target + ".3"));
        }});
        p("planted link", new Probe() { public boolean run() throws Throwable {
            // The Go test leaves a symbolic link to the secret here where the platform allows it.
            new java.io.FileInputStream(new java.io.File(System.getProperty("dvb.persistent.root"), "link.xml"));
            return true;
        }});
        p("windows share name", new Probe() { public boolean run() throws Throwable {
            new java.io.FileInputStream("\\\\127.0.0.1\\wms-probe\\x");
            return true;
        }});
        p("windows device name", new Probe() { public boolean run() throws Throwable {
            new java.io.FileOutputStream("\\\\.\\NUL");
            return true;
        }});
        p("file url", new Probe() { public boolean run() throws Throwable {
            new java.net.URL(secretUri).openStream();
            return true;
        }});
        p("resource climb", new Probe() { public boolean run() throws Throwable {
            return getClass().getClassLoader().getResourceAsStream("../../../../" + secret) != null;
        }});
        p("nio files", new Probe() { public boolean run() throws Throwable { return Blocked.nio(secret); }});
        p("file descriptor", new Probe() { public boolean run() throws Throwable { return Blocked.descriptor(); }});
        p("zip file", new Probe() { public boolean run() throws Throwable { return Blocked.zipFile(secret); }});
        p("print stream to a file", new Probe() { public boolean run() throws Throwable { return Blocked.printStream(target + ".4"); }});
        // --- Class loading and reflection.
        p("forName runtime", new Probe() { public boolean run() throws Throwable { return Class.forName("java.lang.Runtime") != null; }});
        p("forName unsafe", new Probe() { public boolean run() throws Throwable { return Class.forName("sun.misc.Unsafe") != null; }});
        p("forName internal", new Probe() { public boolean run() throws Throwable { return Class.forName("jdk.internal.misc.Unsafe") != null; }});
        p("forName host", new Probe() { public boolean run() throws Throwable { return Class.forName("witty.bdj.Host") != null; }});
        p("link host", new Probe() { public boolean run() throws Throwable { return Blocked.host(); }});
        p("loader to runtime", new Probe() { public boolean run() throws Throwable { return getClass().getClassLoader().loadClass("java.lang.ProcessBuilder") != null; }});
        p("system loader", new Probe() { public boolean run() throws Throwable { return ClassLoader.getSystemClassLoader().loadClass("java.lang.ProcessBuilder") != null; }});
        p("loader parent", new Probe() { public boolean run() throws Throwable { return getClass().getClassLoader().getParent() != null; }});
        p("context loader", new Probe() { public boolean run() throws Throwable { return Thread.currentThread().getContextClassLoader().loadClass("java.lang.ProcessBuilder") != null; }});
        p("own loader", new Probe() { public boolean run() throws Throwable { return Blocked.ownLoader(); }});
        p("reflection", new Probe() { public boolean run() throws Throwable { return Blocked.reflect(); }});
        p("method handles", new Probe() { public boolean run() throws Throwable { return Blocked.handles(); }});
        p("proxy", new Probe() { public boolean run() throws Throwable { return Blocked.proxy(); }});
        p("serial", new Probe() { public boolean run() throws Throwable { return Blocked.serial(); }});
        // --- Processes and the machine.
        p("process builder", new Probe() { public boolean run() throws Throwable { return Blocked.process(); }});
        p("runtime exec", new Probe() { public boolean run() throws Throwable { return Blocked.exec(); }});
        p("native library", new Probe() { public boolean run() throws Throwable { System.loadLibrary("net"); return true; }});
        p("environment", new Probe() { public boolean run() throws Throwable { return System.getenv("PATH") != null || System.getProperty("java.home") != null || System.getProperty("java.class.path") != null || System.getProperty("user.home") != null; }});
        p("exit", new Probe() { public boolean run() throws Throwable { System.exit(3); Runtime.getRuntime().exit(3); Runtime.getRuntime().halt(3); return false; }});
        p("desktop", new Probe() { public boolean run() throws Throwable { return Blocked.desktop(); }});
    }

    /** All the text of a parsed document. */
    private static String textOf(org.w3c.dom.Document d) { return String.valueOf(d.getDocumentElement().getTextContent()); }

    private static final class Grab extends org.xml.sax.helpers.DefaultHandler {
        final StringBuffer text = new StringBuffer();
        public void characters(char[] c, int o, int l) { text.append(c, o, l); }
    }
}
