package javax.media;
public interface Controller extends Clock, Duration {
    Time LATENCY_UNKNOWN = new Time(Long.MAX_VALUE);
    int Unrealized = 100, Realizing = 200, Realized = 300, Prefetching = 400, Prefetched = 500, Started = 600;
    int getState();
    int getTargetState();
    void realize();
    void prefetch();
    void deallocate();
    void close();
    Time getStartLatency();
    Control[] getControls();
    Control getControl(String type);
    void addControllerListener(ControllerListener l);
    void removeControllerListener(ControllerListener l);
}
