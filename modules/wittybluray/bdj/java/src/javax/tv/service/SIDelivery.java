package javax.tv.service;
import java.util.concurrent.atomic.AtomicInteger;

/** Platform helper: delivers one SI result to a requestor on its own short thread, as the spec asks, and lets the request be cancelled. */
public final class SIDelivery implements SIRequest, Runnable {
    private static final AtomicInteger live = new AtomicInteger();
    private final SIRequestor to;
    private final SIRetrievable[] ok;
    private final SIRequestFailureType bad;
    private boolean claimed, counted;

    private SIDelivery(SIRequestor r, SIRetrievable[] v, SIRequestFailureType f) { to = r; ok = v; bad = f; }
    public static SIRequest success(SIRequestor r, SIRetrievable[] v) { return start(new SIDelivery(r, v, null)); }
    public static SIRequest failure(SIRequestor r, SIRequestFailureType f) { return start(new SIDelivery(r, null, f)); }

    private static SIRequest start(SIDelivery d) {
        if (d.to == null) return d;
        if (live.incrementAndGet() > 32) {
            live.decrementAndGet();
            d.run();
            return d;
        }
        d.counted = true;
        try {
            Thread t = new Thread(d, "si-request");
            t.setDaemon(true);
            t.start();
        } catch (Throwable e) {
            live.decrementAndGet();
            d.counted = false;
            d.run();
        }
        return d;
    }
    private synchronized boolean claim() {
        if (claimed) return false;
        claimed = true;
        return true;
    }
    public boolean cancel() { return claim(); }
    public void run() {
        try {
            if (!claim()) return;
            if (ok != null) to.notifySuccess(ok);
            else to.notifyFailure(bad);
        } catch (ThreadDeath d) {
            throw d;
        } catch (Throwable e) {
            e.printStackTrace();
        } finally {
            if (counted) live.decrementAndGet();
        }
    }
}
