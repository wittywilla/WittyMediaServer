package a;

import java.util.ArrayList;
import java.util.List;
import org.bluray.system.RegisterAccess;

/** The disc that tries to bring the server down, not to escape: one mode per run. */
final class Hostile {
    private Hostile() {}

    static void go(String mode) {
        if (mode.equals("spin")) {
            for (int i = 0; i < 64; i++) new Thread(new Runnable() { public void run() { for (;;) { } } }).start();
        } else if (mode.equals("heap")) {
            List<byte[]> l = new ArrayList<byte[]>();
            for (;;) l.add(new byte[1 << 20]);
        } else if (mode.equals("direct")) {
            List<java.nio.ByteBuffer> l = new ArrayList<java.nio.ByteBuffer>();
            for (;;) l.add(java.nio.ByteBuffer.allocateDirect(1 << 20));
        } else if (mode.equals("native")) {
            // Each of these holds native memory the Java heap does not count.
            List<java.util.zip.Deflater> l = new ArrayList<java.util.zip.Deflater>();
            for (;;) l.add(new java.util.zip.Deflater());
        } else if (mode.equals("threads")) {
            for (;;) {
                new Thread(new Runnable() {
                    public void run() {
                        try {
                            Thread.sleep(600000);
                        } catch (InterruptedException e) {
                            return;
                        }
                    }
                }).start();
            }
        } else if (mode.equals("flood-play")) {
            for (;;) {
                try {
                    javax.media.Manager.createPlayer(new javax.media.MediaLocator("bd://1.PLAYLIST:00001")).start();
                } catch (Exception e) {
                    return;
                }
            }
        } else if (mode.equals("flood-title")) {
            for (;;) {
                try {
                    javax.tv.service.selection.ServiceContextFactory.getInstance().createServiceContext().select(new org.bluray.ti.Title(1));
                } catch (Exception e) {
                    return;
                }
            }
        } else if (mode.equals("flood-gpr")) {
            for (int i = 0; ; i++) RegisterAccess.getInstance().setGPR(i & 4095, i);
        } else if (mode.equals("flood-log")) {
            char[] line = new char[100000];
            java.util.Arrays.fill(line, 'x');
            String s = new String(line);
            for (;;) System.out.println(s);
        } else if (mode.equals("flood-sound")) {
            for (;;) {
                try {
                    javax.media.Manager.createPlayer(new javax.media.MediaLocator("bd://SOUND:01")).start();
                } catch (Exception e) {
                    return;
                }
            }
        } else if (mode.equals("disk")) {
            String dir = System.getProperty("dvb.persistent.root");
            byte[] b = new byte[1 << 20];
            try {
                java.io.FileOutputStream o = new java.io.FileOutputStream(new java.io.File(dir, "big.bin"));
                for (;;) o.write(b);
            } catch (java.io.IOException e) {
                RegisterAccess.getInstance().setGPR(198, 1);
            }
        } else if (mode.equals("stack")) {
            down(0);
        } else if (mode.equals("hang-paint")) {
            org.havi.ui.HScene s = org.havi.ui.HSceneFactory.getInstance().getDefaultHScene();
            java.awt.Component c = new java.awt.Component() {
                public void paint(java.awt.Graphics g) { for (;;) { } }
            };
            c.setBounds(0, 0, 10, 10);
            s.add(c);
            s.repaint();
        } else if (mode.equals("kill-threads")) {
            // Interrupts every thread it can find, the platform's own too.
            ThreadGroup g = Thread.currentThread().getThreadGroup();
            while (g.getParent() != null) g = g.getParent();
            Thread[] all = new Thread[256];
            int n = g.enumerate(all);
            for (int i = 0; i < n; i++) if (all[i] != Thread.currentThread()) all[i].interrupt();
            RegisterAccess.getInstance().setGPR(198, n);
        }
    }

    private static int down(int n) { return down(n + 1) + 1; }
}
