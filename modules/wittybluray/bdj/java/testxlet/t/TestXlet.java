package t;

import java.awt.Color;
import java.awt.Component;
import java.awt.Graphics;
import java.awt.event.KeyAdapter;
import java.awt.event.KeyEvent;
import javax.media.ControllerEvent;
import javax.media.ControllerListener;
import javax.media.EndOfMediaEvent;
import javax.media.Manager;
import javax.media.MediaLocator;
import javax.media.Player;
import javax.tv.xlet.Xlet;
import javax.tv.xlet.XletContext;
import org.bluray.system.RegisterAccess;
import org.dvb.event.EventManager;
import org.dvb.event.UserEvent;
import org.dvb.event.UserEventListener;
import org.dvb.event.UserEventRepository;
import org.havi.ui.HScene;
import org.havi.ui.HSceneFactory;

/**
 * The test disc's application: it draws, takes keys, plays a playlist, stores a
 * file, and tries everything a disc must not be able to do. GPR 100 and up report
 * what happened, for the Go test to read over the pipe (1 = as it should be).
 */
public class TestXlet implements Xlet {
    private static final int OK_CTX = 100, OK_FORNAME = 101, OK_EXEC = 102, OK_FILE = 103, OK_REFLECT = 104, OK_NET = 105, OK_PERSIST = 106, OK_WRITEDISC = 107, OK_RESOURCE = 108, OK_LOADER = 109, OK_END = 110, OK_ENDED = 111;
    private volatile Color color = Color.red;

    private static void report(int gpr, int v) { RegisterAccess.getInstance().setGPR(gpr, v); }

    public void initXlet(XletContext c) {
        String[] args = (String[]) c.getXletProperty(XletContext.ARGS);
        report(OK_CTX, args != null && args.length == 1 && "hello world".equals(args[0]) ? 1 : 2);
    }

    public void startXlet() {
        final HScene s = HSceneFactory.getInstance().getDefaultHScene();
        Component box = new Component() {
            public void paint(Graphics g) {
                g.setColor(color);
                g.fillRect(0, 0, 100, 100);
            }
        };
        box.setBounds(10, 20, 100, 100);
        s.add(box);
        s.setVisible(true);
        box.repaint();
        s.addKeyListener(new KeyAdapter() {
            public void keyPressed(KeyEvent e) {
                if (e.getKeyCode() == KeyEvent.VK_ENTER) {
                    color = Color.green;
                    s.repaint();
                }
            }
        });
        UserEventRepository r = new UserEventRepository("colours");
        r.addAllColourKeys();
        EventManager.getInstance().addUserEventListener(new UserEventListener() {
            public void userEventReceived(UserEvent e) {
                if (e.getType() == KeyEvent.KEY_PRESSED && e.getCode() == 403) {
                    color = Color.blue;
                    s.repaint();
                }
            }
        }, r);
        probes();
        try {
            Player p = Manager.createPlayer(new MediaLocator("bd://1.PLAYLIST:00001"));
            p.addControllerListener(new ControllerListener() {
                public void controllerUpdate(ControllerEvent e) {
                    if (e instanceof EndOfMediaEvent) report(OK_ENDED, 1);
                }
            });
            p.start();
        } catch (Exception e) {
            e.printStackTrace();
        }
        report(OK_END, 1);
    }

    private void probes() {
        // Each probe is in its own method, so a link failure stays inside it.
        try {
            Class.forName("java.lang.ProcessBuilder");
            report(OK_FORNAME, 2);
        } catch (Throwable t) {
            report(OK_FORNAME, 1);
        }
        try {
            exec();
            report(OK_EXEC, 2);
        } catch (Throwable t) {
            report(OK_EXEC, 1);
        }
        // Outside the disc's areas, a file does not exist.
        java.io.File f = new java.io.File(java.lang.System.getProperty("bluray.vfs.root"), "../../../../../../../../../../../Windows/win.ini");
        java.io.File g = new java.io.File("/etc/passwd");
        report(OK_FILE, !f.exists() && !g.exists() ? 1 : 2);
        try {
            reflect();
            report(OK_REFLECT, 2);
        } catch (Throwable t) {
            report(OK_REFLECT, 1);
        }
        try {
            new java.net.Socket("127.0.0.1", 80);
            report(OK_NET, 2);
        } catch (Throwable t) {
            report(OK_NET, 1);
        }
        try {
            java.io.File dir = new java.io.File(java.lang.System.getProperty("dvb.persistent.root"));
            java.io.FileOutputStream o = new java.io.FileOutputStream(new java.io.File(dir, "score.txt"));
            o.write(new byte[] {1, 2, 3});
            o.close();
            report(OK_PERSIST, new java.io.File(dir, "score.txt").length() == 3 ? 1 : 2);
        } catch (Throwable t) {
            report(OK_PERSIST, 2);
        }
        try {
            new java.io.FileOutputStream(new java.io.File(java.lang.System.getProperty("bluray.vfs.root"), "BDMV/evil.txt"));
            report(OK_WRITEDISC, 2);
        } catch (java.io.IOException e) {
            report(OK_WRITEDISC, 1);
        } catch (Throwable t) {
            report(OK_WRITEDISC, 2);
        }
        java.io.InputStream in = getClass().getResourceAsStream("/t/hello.txt");
        java.net.URL u = getClass().getResource("/t/hello.txt");
        report(OK_RESOURCE, in != null && u != null ? 1 : 2);
        try {
            ClassLoader l = getClass().getClassLoader();
            report(OK_LOADER, l.getResourceAsStream("t/hello.txt") != null ? 1 : 2);
        } catch (Throwable t) {
            report(OK_LOADER, 2);
        }
    }

    private static void exec() throws Exception { new ProcessBuilder("cmd").start(); }

    private static void reflect() throws Exception {
        java.lang.reflect.Method m = String.class.getMethod("length");
        m.invoke("x");
    }

    public void pauseXlet() {}
    public void destroyXlet(boolean u) {}
}
