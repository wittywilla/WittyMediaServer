package witty.bdj;

import java.util.ArrayList;
import java.util.List;

/**
 * The one video the player shows. Applications ask for a playlist; the server
 * plays it to the viewer and reports the time, and this keeps the clock between
 * reports, so that marks fire on time without chatter on the link.
 */
public final class Playback {
    private Playback() {}

    public interface Listener {
        void playbackEvent(String what, String[] words);
    }

    private static final List<Listener> listeners = new java.util.concurrent.CopyOnWriteArrayList<Listener>();
    public static int playlist = -1;
    public static double duration;
    public static double[] marks = new double[0];
    public static double[] items = new double[0];
    private static volatile double time;
    private static volatile long timeAt;
    private static volatile boolean running;
    private static volatile double rate = 1;
    private static int nextMark;

    static {
        Host.listen(new Host.Listener() {
            public void onMessage(String[] w) {
                if (w[0].equals("time")) {
                    setClock(Double.parseDouble(w[1]));
                } else if (w[0].equals("ended")) {
                    running = false;
                    fire("ended", w);
                } else if (w[0].equals("plinfo")) {
                    playlist = Integer.parseInt(w[1]);
                    duration = Double.parseDouble(w[2]);
                    marks = list(w.length > 3 ? w[3] : "");
                    items = list(w.length > 4 ? w[4] : "");
                    nextMark = 0;
                    fire("plinfo", w);
                } else if (w[0].equals("playfailed")) {
                    running = false;
                    fire("failed", w);
                }
            }
        });
        Thread t = new Thread(new Runnable() {
            public void run() {
                for (;;) {
                    try {
                        Thread.sleep(100);
                    } catch (InterruptedException e) {
                        continue; // disc code can reach this thread
                    }
                    if (!running) continue;
                    double now = now();
                    while (nextMark < marks.length && marks[nextMark] <= now) {
                        fire("mark", new String[] {"mark", String.valueOf(nextMark)});
                        nextMark++;
                    }
                }
            }
        }, "bdj-marks");
        t.setDaemon(true);
        t.start();
    }

    private static double[] list(String csv) {
        if (csv.length() == 0 || csv.equals("-")) return new double[0];
        String[] p = csv.split(",");
        double[] o = new double[p.length];
        for (int i = 0; i < p.length; i++) o[i] = Double.parseDouble(p[i]);
        return o;
    }

    private static void fire(String what, String[] w) {
        for (Listener l : listeners) {
            try {
                l.playbackEvent(what, w);
            } catch (Throwable t) {
                Host.log("playback listener: " + t);
            }
        }
    }

    private static void setClock(double t) {
        time = t;
        timeAt = java.lang.System.currentTimeMillis();
        int m = 0;
        while (m < marks.length && marks[m] <= t) m++;
        if (m < nextMark || m > nextMark + 1) nextMark = m;
    }

    public static void addListener(Listener l) { listeners.add(l); }
    public static void removeListener(Listener l) { listeners.remove(l); }
    public static double now() {
        double t = time;
        if (running) t += (java.lang.System.currentTimeMillis() - timeAt) / 1000.0 * rate;
        return t;
    }
    public static boolean isRunning() { return running; }

    public static void play(int pl, int item, int mark, double at) {
        running = true;
        setClock(at);
        Host.send("play " + pl + " " + item + " " + mark + " " + at);
    }
    public static void stop() { running = false; Host.send("stop"); }
    public static void pause() { time = now(); running = false; Host.send("pause"); }
    public static void resume() { timeAt = java.lang.System.currentTimeMillis(); running = true; Host.send("resume"); }
    public static void seek(double t) { setClock(t); Host.send("seek " + t); }
    public static void setRate(double r) {
        time = now();
        timeAt = java.lang.System.currentTimeMillis();
        rate = r;
        Host.send("rate " + r);
    }
    public static double getRate() { return rate; }
    public static void videoRect(int x, int y, int w, int h) { Host.send("video " + x + " " + y + " " + w + " " + h); }
}
