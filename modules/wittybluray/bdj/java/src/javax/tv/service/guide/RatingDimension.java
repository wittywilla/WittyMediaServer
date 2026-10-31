package javax.tv.service.guide;
public interface RatingDimension extends javax.tv.service.SIElement {
    String getDimensionName();
    String getRatingRegion();
    int getNumberOfLevels();
    String getRatingLevelDescription(int level);
}
