package witty.bdj;

import java.io.BufferedReader;
import java.io.File;
import java.io.InputStreamReader;
import java.io.PrintStream;
import java.nio.charset.StandardCharsets;

/**
 * Starts the runtime: java -cp runtime.jar witty.bdj.Main vfs-root persist-root temp-root [probe].
 * Standard output is the link to the server, so anything else that prints goes
 * to standard error.
 */
public final class Main {
    private Main() {}

    public static void main(String[] args) throws Exception {
        java.lang.System.setProperty("java.awt.headless", "true");
        PrintStream real = java.lang.System.out;
        java.lang.System.setOut(java.lang.System.err);
        Host.open(real);
        if (args.length >= 4 && args[3].equals("probe")) {
            // Prove this Java can do what a disc needs: draw, off screen, with fonts.
            java.awt.image.BufferedImage img = new java.awt.image.BufferedImage(16, 16, java.awt.image.BufferedImage.TYPE_INT_ARGB);
            java.awt.Graphics2D g = img.createGraphics();
            g.setFont(new java.awt.Font("SansSerif", java.awt.Font.PLAIN, 10));
            g.drawString("x", 1, 10);
            g.dispose();
            Host.send("ok " + java.lang.System.getProperty("java.version"));
            java.lang.System.exit(0);
        }
        if (args.length >= 4 && args[3].equals("refused")) {
            // For the diagnostic: which of the JDK classes named on standard input a disc may not link.
            BufferedReader names = new BufferedReader(new InputStreamReader(java.lang.System.in, StandardCharsets.UTF_8));
            for (String n; (n = names.readLine()) != null; ) {
                if (!Sandbox.RENAME.containsKey(n) && !Sandbox.allowedJdk(n.replace('/', '.'))) Host.send("refused " + n);
            }
            java.lang.System.exit(0);
        }
        NoNetwork.install();
        Host.vfsRoot = new File(args[0]).getCanonicalPath();
        Host.persistRoot = new File(args[1]).getCanonicalPath();
        Host.tempRoot = new File(args[2]).getCanonicalPath();
        Plane.start();
        Apps.init();
        Thread reader = new Thread(new Runnable() {
            public void run() {
                try {
                    BufferedReader in = new BufferedReader(new InputStreamReader(java.lang.System.in, StandardCharsets.UTF_8));
                    for (String line; (line = in.readLine()) != null; ) {
                        String[] w = line.split(" ");
                        if (w.length == 0 || w[0].length() == 0) continue;
                        if (w[0].equals("quit")) break;
                        Host.dispatch(w);
                    }
                } catch (Throwable t) {
                    t.printStackTrace();
                }
                java.lang.System.exit(0); // no server, no reason to run
            }
        }, "bdj-link");
        reader.start();
        Host.send("ready");
    }
}
