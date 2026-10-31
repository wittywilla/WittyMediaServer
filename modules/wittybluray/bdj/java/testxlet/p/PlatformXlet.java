package p;

import java.awt.Color;
import java.awt.Dimension;
import java.awt.Font;
import java.awt.Graphics;
import java.awt.Graphics2D;
import java.awt.Image;
import java.awt.Toolkit;
import java.io.ByteArrayInputStream;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.RandomAccessFile;
import javax.tv.xlet.Xlet;
import javax.tv.xlet.XletContext;
import org.bluray.net.BDLocator;
import org.bluray.system.RegisterAccess;
import org.dvb.ui.DVBBufferedImage;
import org.havi.ui.HScene;
import org.havi.ui.HSceneFactory;
import org.havi.ui.HScreen;
import org.havi.ui.event.HRcCapabilities;
import org.havi.ui.event.HRcEvent;

/**
 * What real discs' code does, one call at a time: the calls this class makes have
 * each broken, or could break, on the way through the sandbox's class rewriting.
 * Results go out as a GPR per check, as in the hostile disc: 1 = it worked.
 * GPR 299 is set when all have run.
 */
public class PlatformXlet implements Xlet {
    public static final int FIRST = 300, DONE = 299;
    private XletContext ctx;
    private int next = FIRST;

    private interface Check {
        boolean run() throws Throwable;
    }

    private static void report(int gpr, int v) { RegisterAccess.getInstance().setGPR(gpr, v); }

    private void c(String name, Check x) {
        int gpr = next++;
        boolean ok = false;
        String why = "returned false";
        try {
            ok = x.run();
        } catch (Throwable t) {
            why = t.toString();
        }
        System.out.println("check " + gpr + " " + name + ": " + (ok ? "ok" : why));
        report(gpr, ok ? 1 : 2);
    }

    public void initXlet(XletContext x) { ctx = x; }
    public void pauseXlet() {}
    public void destroyXlet(boolean u) { report(298, 1); }

    public void startXlet() {
        checks();
        report(DONE, 1);
    }

    private void checks() {
        final String persist = System.getProperty("dvb.persistent.root");
        // --- What a menu asks for first.
        c("default screen", new Check() { public boolean run() {
            return HScreen.getDefaultHScreen().getDefaultHGraphicsDevice().getCurrentConfiguration() != null
                && HScreen.getDefaultHScreen().getDefaultHVideoDevice().getCurrentConfiguration() != null
                && HScreen.getDefaultHScreen().getDefaultHBackgroundDevice() != null;
        }});
        c("rc capabilities", new Check() { public boolean run() { return HRcCapabilities.isSupported(HRcEvent.VK_COLORED_KEY_0) && HRcCapabilities.getRepresentation(HRcEvent.VK_COLORED_KEY_0) != null; }});
        c("locator forms", new Check() { public boolean run() throws Throwable {
            BDLocator l = new BDLocator("bd://1.PLAYLIST:00002.MARK:00003");
            BDLocator s = new BDLocator("bd://SOUND:01");
            return l.getTitleNumber() == 1 && l.getPlayListId() == 2 && l.getMarkId() == 3 && s.getSoundId() == 1;
        }});
        c("service manager", new Check() { public boolean run() throws Throwable {
            return javax.tv.service.SIManager.createInstance().getService(new BDLocator("bd://1")) instanceof org.bluray.ti.Title;
        }});
        c("disc manager", new Check() { public boolean run() { return org.bluray.ti.DiscManager.getDiscManager().getCurrentDisc() != null; }});
        c("timer", new Check() { public boolean run() throws Throwable {
            final Object done = new Object();
            final boolean[] fired = new boolean[1];
            javax.tv.util.TVTimerSpec s = new javax.tv.util.TVTimerSpec();
            s.setDelayTime(5);
            s.addTVTimerWentOffListener(new javax.tv.util.TVTimerWentOffListener() {
                public void timerWentOff(javax.tv.util.TVTimerWentOffEvent e) {
                    synchronized (done) {
                        fired[0] = true;
                        done.notifyAll();
                    }
                }
            });
            javax.tv.util.TVTimer.getTimer().scheduleTimerSpec(s);
            synchronized (done) {
                for (int i = 0; i < 100 && !fired[0]; i++) done.wait(100);
            }
            return fired[0];
        }});
        c("user preferences", new Check() { public boolean run() throws Throwable {
            org.dvb.user.GeneralPreference p = new org.dvb.user.GeneralPreference("User Language");
            org.dvb.user.UserPreferenceManager.getInstance().read(p);
            return true;
        }});
        c("apps database", new Check() { public boolean run() {
            return org.dvb.application.AppsDatabase.getAppsDatabase().size() > 0;
        }});
        // --- Files, the way discs name them.
        c("storage home", new Check() { public boolean run() throws Throwable {
            File f = new File(persist + "/" + ctx.getXletProperty("dvb.org.id") + "/" + ctx.getXletProperty("dvb.app.id") + "/tmp.txt");
            FileOutputStream o = new FileOutputStream(f);
            o.write(new byte[] {1, 2, 3});
            o.close();
            return f.length() == 3 && f.toURI() != null && f.toURL() != null && new FileInputStream(f).read() == 1;
        }});
        c("random access", new Check() { public boolean run() throws Throwable {
            RandomAccessFile r = new RandomAccessFile(new File(persist, "r.bin"), "rw");
            r.writeInt(7);
            r.writeBytes("ab");
            r.seek(0);
            int v = r.readInt();
            r.setLength(2);
            long n = r.length();
            r.close();
            return v == 7 && n == 2;
        }});
        c("storage has a limit", new Check() { public boolean run() throws Throwable {
            RandomAccessFile r = new RandomAccessFile(new File(persist, "huge.bin"), "rw");
            try {
                r.setLength(1L << 40);
                return false;
            } catch (java.io.IOException e) {
                return r.length() == 0;
            } finally {
                r.close();
            }
        }});
        c("hole past the limit", new Check() { public boolean run() throws Throwable {
            RandomAccessFile r = new RandomAccessFile(new File(persist, "hole.bin"), "rw");
            try {
                r.seek(1L << 40);
                r.write(1);
                return false;
            } catch (java.io.IOException e) {
                return true;
            } finally {
                r.close();
            }
        }});
        c("own resource by url", new Check() { public boolean run() throws Throwable {
            java.net.URL u = getClass().getResource("/p/doc.xml");
            return u != null && u.openStream().read() >= 0 && new java.net.URL(u.toExternalForm()) != null;
        }});
        c("loading another of the disc's jars", new Check() { public boolean run() throws Throwable {
            String vfs = System.getProperty("bluray.vfs.root").replace('\\', '/');
            org.dvb.lang.DVBClassLoader l = org.dvb.lang.DVBClassLoader.newInstance(new java.net.URL[] {new File(vfs + "/BDMV/JAR/00000.jar").toURL()});
            return l.loadClass("p.PlatformXlet") == PlatformXlet.class && l.getResourceAsStream("p/doc.xml") != null;
        }});
        c("disc's file by mount", new Check() { public boolean run() throws Throwable {
            org.dvb.dsmcc.ServiceDomain d = new org.dvb.dsmcc.ServiceDomain();
            d.attach(new BDLocator("bd://JAR:00000"));
            org.dvb.dsmcc.DSMCCObject o = new org.dvb.dsmcc.DSMCCObject(d.getMountPoint(), "BDMV/index.bdmv");
            o.synchronousLoad();
            return o.isLoaded() && o.exists();
        }});
        c("io persistent", new Check() { public boolean run() throws Throwable {
            File f = new File(persist, "attr.txt");
            new FileOutputStream(f).close();
            return org.dvb.io.persistent.FileAttributes.getFileAttributes(f) != null;
        }});
        // --- XML, the way a menu reads its own data.
        c("dom", new Check() { public boolean run() throws Throwable {
            org.w3c.dom.Document d = javax.xml.parsers.DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(getClass().getResourceAsStream("/p/doc.xml"));
            return "menu".equals(d.getDocumentElement().getNodeName()) && d.getElementsByTagName("item").getLength() == 2;
        }});
        c("sax", new Check() { public boolean run() throws Throwable {
            final int[] n = new int[1];
            javax.xml.parsers.SAXParserFactory.newInstance().newSAXParser().parse(getClass().getResourceAsStream("/p/doc.xml"), new org.xml.sax.helpers.DefaultHandler() {
                public void startElement(String u, String l, String q, org.xml.sax.Attributes a) { n[0]++; }
            });
            return n[0] == 3;
        }});
        c("sax reader", new Check() { public boolean run() throws Throwable {
            final int[] n = new int[1];
            org.xml.sax.XMLReader r = javax.xml.parsers.SAXParserFactory.newInstance().newSAXParser().getXMLReader();
            r.setContentHandler(new org.xml.sax.helpers.DefaultHandler() {
                public void startElement(String u, String l, String q, org.xml.sax.Attributes a) { n[0]++; }
            });
            r.parse(new org.xml.sax.InputSource(getClass().getResourceAsStream("/p/doc.xml")));
            return n[0] == 3;
        }});
        c("doctype that fetches nothing", new Check() { public boolean run() throws Throwable {
            String doc = "<!DOCTYPE menu SYSTEM \"http://127.0.0.1:1/x.dtd\"><menu/>";
            return javax.xml.parsers.DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(new ByteArrayInputStream(doc.getBytes("UTF-8"))) != null;
        }});
        // --- What the renaming must not break.
        c("printing a stack trace", new Check() { public boolean run() {
            new Exception("expected").printStackTrace(System.out);
            new java.io.PrintWriter(new java.io.StringWriter()).flush();
            return true;
        }});
        c("listing properties", new Check() { public boolean run() {
            new java.util.Properties().list(System.out);
            return true;
        }});
        c("regular expressions", new Check() { public boolean run() { return java.util.regex.Pattern.compile("a+").matcher("aa").matches(); }});
        c("media by url", new Check() { public boolean run() throws Throwable {
            return javax.media.Manager.createPlayer(new javax.media.MediaLocator(new java.net.URL("bd://1.PLAYLIST:00001"))) != null
                && javax.media.Manager.createPlayer(new java.net.URL("bd://1.PLAYLIST:00001")) != null;
        }});
        c("fonts", new Check() { public boolean run() throws Throwable { return new Font("SansSerif", Font.BOLD, 24).getSize() == 24; }});
        c("toolkit image", new Check() { public boolean run() {
            Image i = Toolkit.getDefaultToolkit().getImage(getClass().getResource("/p/dot.png"));
            return i != null && i.getWidth(null) == 2 && i.getHeight(null) == 2;
        }});
        c("buffered image", new Check() { public boolean run() {
            DVBBufferedImage b = new DVBBufferedImage(8, 8);
            b.createGraphics().dispose();
            b.dispose();
            return true;
        }});
        // --- Drawing straight to the screen, as most animated menus do.
        c("direct drawing", new Check() { public boolean run() {
            HScene s = HSceneFactory.getInstance().getDefaultHScene();
            java.awt.Component box = new java.awt.Component() {
                public void paint(Graphics g) {}
            };
            box.setBounds(new java.awt.Rectangle(new Dimension(1920, 1080)));
            s.add(box);
            s.setVisible(true);
            Graphics2D g = (Graphics2D) box.getGraphics();
            g.setComposite(java.awt.AlphaComposite.Src);
            g.setColor(Color.magenta);
            g.fillRect(100, 100, 50, 50);
            Toolkit.getDefaultToolkit().sync();
            return true;
        }});
    }
}
