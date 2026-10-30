package javax.media;
public class Time {
    public static final long ONE_SECOND = 1000000000L;
    protected long nanoseconds;
    public Time(long n) { nanoseconds = n; }
    public Time(double s) { nanoseconds = (long) (s * ONE_SECOND); }
    public long getNanoseconds() { return nanoseconds; }
    public double getSeconds() { return nanoseconds / (double) ONE_SECOND; }
}
