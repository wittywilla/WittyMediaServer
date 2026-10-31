package javax.tv.service.selection;
public class AlternativeContentEvent extends ServiceContextEvent {
    public static final int CA_REFUSAL = 2, CONTENT_NOT_FOUND = 3, MISSING_HANDLER = 4, TUNING_FAILURE = 5, RATING_PROBLEM = 7, OTHER = 255;
    private final int r;
    public AlternativeContentEvent(ServiceContext s, int reason) { super(s); r = reason; }
    public int getReason() { return r; }
}
