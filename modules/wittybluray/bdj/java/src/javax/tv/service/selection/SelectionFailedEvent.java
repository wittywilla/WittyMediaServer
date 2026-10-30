package javax.tv.service.selection;
public class SelectionFailedEvent extends ServiceContextEvent {
    public static final int INTERRUPTED = 1, CA_REFUSAL = 2, CONTENT_NOT_FOUND = 3, MISSING_HANDLER = 4, TUNING_FAILURE = 5, INSUFFICIENT_RESOURCES = 6, OTHER = 255;
    private final int r;
    public SelectionFailedEvent(ServiceContext s, int reason) { super(s); r = reason; }
    public int getReason() { return r; }
}
