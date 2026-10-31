package javax.tv.util;
import java.util.HashMap;
import java.util.Map;
import java.util.Timer;
import java.util.TimerTask;
import java.util.WeakHashMap;

/** One timer per application (thread group), so a slow listener only delays its own application. The thread exists only while something is scheduled. */
public abstract class TVTimer {
    private static final Map<ThreadGroup, TVTimer> byGroup = new WeakHashMap<ThreadGroup, TVTimer>();
    protected TVTimer() {}
    public static TVTimer getTimer() {
        ThreadGroup g = Thread.currentThread().getThreadGroup();
        synchronized (byGroup) {
            TVTimer t = byGroup.get(g);
            if (t == null) byGroup.put(g, t = new Impl());
            return t;
        }
    }
    public abstract void scheduleTimerSpec(TVTimerSpec spec) throws TVTimerScheduleFailedException;
    public abstract void deschedule(TVTimerSpec spec);
    public abstract long getMinRepeatInterval();
    public abstract long getGranularity();

    private static final class Impl extends TVTimer {
        private static final int MAX = 256;
        private static final long MIN_REPEAT = 20;
        private final Map<TVTimerSpec, TimerTask> tasks = new HashMap<TVTimerSpec, TimerTask>();
        private Timer timer;

        public long getMinRepeatInterval() { return MIN_REPEAT; }
        public long getGranularity() { return 1; }

        public synchronized void scheduleTimerSpec(TVTimerSpec s) throws TVTimerScheduleFailedException {
            if (s == null) throw new TVTimerScheduleFailedException("no timer spec");
            boolean again = !s.isAbsolute() && s.isRepeat();
            if (again && s.getTime() < MIN_REPEAT) throw new TVTimerScheduleFailedException("repeat interval below " + MIN_REPEAT + " ms");
            TimerTask old = tasks.remove(s);
            if (old != null) old.cancel();
            if (tasks.size() >= MAX) throw new TVTimerScheduleFailedException("too many timers");
            long delay = s.isAbsolute() ? s.getTime() - System.currentTimeMillis() : s.getTime();
            if (delay < 0) delay = 0;
            for (int attempt = 0; ; attempt++) {
                Task t = new Task(s, again);
                try {
                    if (timer == null) timer = new Timer("tvtimer", true);
                    if (!again) timer.schedule(t, delay);
                    else if (s.isRegular()) timer.scheduleAtFixedRate(t, delay, s.getTime());
                    else timer.schedule(t, delay, s.getTime());
                    tasks.put(s, t);
                    return;
                } catch (IllegalStateException e) { // the timer thread was killed: start a fresh one
                    timer = null;
                    tasks.clear();
                    if (attempt > 0) throw new TVTimerScheduleFailedException("timer unavailable");
                }
            }
        }
        public synchronized void deschedule(TVTimerSpec s) {
            TimerTask t = s == null ? null : tasks.remove(s);
            if (t != null) t.cancel();
            idle();
        }
        private void idle() {
            if (tasks.isEmpty() && timer != null) {
                timer.cancel();
                timer = null;
            }
        }

        private final class Task extends TimerTask {
            private final TVTimerSpec spec;
            private final boolean again;
            Task(TVTimerSpec s, boolean a) { spec = s; again = a; }
            public void run() {
                synchronized (Impl.this) {
                    if (tasks.get(spec) != this) return;
                    if (!again) {
                        tasks.remove(spec);
                        idle();
                    }
                }
                spec.notifyListeners(Impl.this);
            }
        }
    }
}
