package javax.media;
public interface Clock {
    Time RESET = new Time(Long.MAX_VALUE);
    void setTimeBase(TimeBase b) throws IncompatibleTimeBaseException;
    void syncStart(Time t);
    void stop();
    void setStopTime(Time t);
    Time getStopTime();
    void setMediaTime(Time t);
    Time getMediaTime();
    long getMediaNanoseconds();
    Time getSyncTime();
    TimeBase getTimeBase();
    Time mapToTimeBase(Time t) throws ClockStoppedException;
    float getRate();
    float setRate(float r);
}
