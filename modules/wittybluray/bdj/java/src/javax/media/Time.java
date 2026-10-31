package javax.media;
public class Time {
    public static final long ONE_SECOND = 1000000000L;
    public static final Time TIME_UNKNOWN = new Time(Long.MAX_VALUE - 1);
    protected long nanoseconds;
    public Time(long n) { nanoseconds = n; }
    public Time(double s) { nanoseconds = secondsToNanoseconds(s); }
    public static long secondsToNanoseconds(double s) { return (long) (s * ONE_SECOND); }
    public long getNanoseconds() { return nanoseconds; }
    public double getSeconds() { return nanoseconds / (double) ONE_SECOND; }
}
