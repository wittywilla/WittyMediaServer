package javax.tv.service.selection;
public class PresentationTerminatedEvent extends ServiceContextEvent {
    public static final int SERVICE_VANISHED = 1, TUNED_AWAY = 2, RESOURCES_REMOVED = 3, ACCESS_WITHDRAWN = 4, USER_STOP = 5, OTHER = 255;
    private final int r;
    public PresentationTerminatedEvent(ServiceContext s, int reason) { super(s); r = reason; }
    public int getReason() { return r; }
}
