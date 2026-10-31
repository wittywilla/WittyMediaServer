package javax.tv.util;
/** What a timer should do: a time in milliseconds (absolute, or a delay), whether it repeats, and who to tell. */
public class TVTimerSpec {
    private volatile long time;
    private volatile boolean absolute, repeat, regular;
    private final java.util.List<TVTimerWentOffListener> ls = new java.util.concurrent.CopyOnWriteArrayList<TVTimerWentOffListener>();
    public TVTimerSpec() {}
    public void setAbsolute(boolean a) { absolute = a; }
    public boolean isAbsolute() { return absolute; }
    public void setAbsoluteTime(long t) { time = t; absolute = true; }
    public void setDelayTime(long t) { time = t; absolute = false; }
    public void setTime(long t) { time = t; }
    public long getTime() { return time; }
    public void setRepeat(boolean r) { repeat = r; }
    public boolean isRepeat() { return repeat; }
    public void setRegular(boolean r) { regular = r; }
    public boolean isRegular() { return regular; }
    public void addTVTimerWentOffListener(TVTimerWentOffListener l) { if (l != null && !ls.contains(l)) ls.add(l); }
    public void removeTVTimerWentOffListener(TVTimerWentOffListener l) { ls.remove(l); }
    public void notifyListeners(TVTimer source) {
        TVTimerWentOffEvent e = new TVTimerWentOffEvent(source, this);
        for (TVTimerWentOffListener l : ls) {
            try {
                l.timerWentOff(e);
            } catch (ThreadDeath d) {
                throw d;
            } catch (Throwable t) {
                t.printStackTrace();
            }
        }
    }
    public String toString() { return "TVTimerSpec[" + (absolute ? "at " : "in ") + time + "ms" + (repeat ? (regular ? ", regular repeat" : ", repeat") : "") + "]"; }
}
