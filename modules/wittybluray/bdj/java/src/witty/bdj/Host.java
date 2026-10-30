package witty.bdj;

import java.io.BufferedOutputStream;
import java.io.DataOutputStream;
import java.io.File;
import java.io.IOException;
import java.io.InputStream;
import java.io.PrintStream;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Collections;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CopyOnWriteArrayList;

/**
 * The link to the server: the only thing writing to the real standard output.
 * Messages each way are a line of space-separated words; a frame adds raw pixels.
 * The server is the player: it owns the registers, the video and the viewer.
 */
public final class Host {
    private Host() {}

    private static DataOutputStream out;
    public static final int[] PSR = new int[128];
    public static final int[] GPR = new int[4096];
    private static final List<Listener> listeners = new CopyOnWriteArrayList<Listener>();
    public static String vfsRoot = "", persistRoot = "", tempRoot = "";
    public static int width = 1920, height = 1080;
    public static long started = java.lang.System.currentTimeMillis();

    /** Something that wants to hear what the server says. */
    public interface Listener {
        void onMessage(String[] words);
    }

    public static void listen(Listener l) { listeners.add(l); }

    static void open(java.io.OutputStream o) { out = new DataOutputStream(new BufferedOutputStream(o, 1 << 16)); }

    public static void send(String line) {
        synchronized (Host.class) {
            try {
                out.write(line.getBytes(StandardCharsets.UTF_8));
                out.write('\n');
                out.flush();
            } catch (IOException e) {
                java.lang.System.exit(0); // the server has gone
            }
        }
    }

    /** One changed rectangle of the graphics plane, as big-endian ARGB. */
    public static void sendFrame(int x, int y, int w, int h, int[] argb, int off, int stride) {
        synchronized (Host.class) {
            try {
                out.write(("frame " + x + " " + y + " " + w + " " + h + "\n").getBytes(StandardCharsets.UTF_8));
                for (int j = 0; j < h; j++) {
                    int o = off + j * stride;
                    for (int i = 0; i < w; i++) out.writeInt(argb[o + i]);
                }
                out.flush();
            } catch (IOException e) {
                java.lang.System.exit(0);
            }
        }
    }

    public static void log(String s) { send("log " + s.replace('\n', ' ')); }

    static void dispatch(String[] w) {
        if (w[0].equals("psr")) {
            PSR[Integer.parseInt(w[1]) & 127] = (int) Long.parseLong(w[2]);
        } else if (w[0].equals("gpr")) {
            GPR[Integer.parseInt(w[1]) & 4095] = (int) Long.parseLong(w[2]);
        }
        for (Listener l : listeners) {
            try {
                l.onMessage(w);
            } catch (Throwable t) {
                log("listener: " + t);
            }
        }
    }

    public static void setPSR(int n, int v) {
        PSR[n & 127] = v;
        send("psr " + (n & 127) + " " + (v & 0xffffffffL));
    }

    public static void setGPR(int n, int v) {
        GPR[n & 4095] = v;
        send("gpr " + (n & 4095) + " " + (v & 0xffffffffL));
    }
}
