package javax.media;
public class SystemTimeBase implements TimeBase {
    public SystemTimeBase() {}
    public Time getTime() { return new Time(getNanoseconds()); }
    public long getNanoseconds() { return java.lang.System.nanoTime(); }
}
