package javax.tv.service.guide;
public interface ProgramEvent extends javax.tv.service.SIElement {
    String getName();
    java.util.Date getStartTime();
    java.util.Date getEndTime();
    long getDuration();
    ContentRatingAdvisory getRating();
    javax.tv.service.SIRequest retrieveDescription(javax.tv.service.SIRequestor r);
}
