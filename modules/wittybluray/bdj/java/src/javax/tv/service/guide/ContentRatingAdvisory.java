package javax.tv.service.guide;
public interface ContentRatingAdvisory {
    String getDisplayText();
    String getRatingRegion();
    String[] getDimensionNames();
    String getRatingDescription(String dimensionName);
}
