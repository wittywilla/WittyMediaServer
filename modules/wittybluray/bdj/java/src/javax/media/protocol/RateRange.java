package javax.media.protocol;
public class RateRange implements java.io.Serializable {
    private float current;
    private final float min, max;
    private final boolean exact;
    public RateRange(RateRange r) { this(r.current, r.min, r.max, r.exact); }
    public RateRange(float init, float min, float max, boolean exact) { current = init; this.min = min; this.max = max; this.exact = exact; }
    public float setCurrentRate(float rate) {
        current = rate < min ? min : rate > max ? max : rate;
        return current;
    }
    public float getCurrentRate() { return current; }
    public float getMinimumRate() { return min; }
    public float getMaximumRate() { return max; }
    public boolean isExact() { return exact; }
}
