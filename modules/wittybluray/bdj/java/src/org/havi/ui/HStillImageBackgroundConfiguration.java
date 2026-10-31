package org.havi.ui;
public class HStillImageBackgroundConfiguration extends HBackgroundConfiguration {
    protected HStillImageBackgroundConfiguration() {}
    public void displayImage(HBackgroundImage i) throws java.io.IOException, HPermissionDeniedException, HConfigurationException { displayImage(i, null); }
    public void displayImage(HBackgroundImage i, HScreenRectangle r) throws java.io.IOException, HPermissionDeniedException, HConfigurationException {
        if (i == null) throw new NullPointerException();
        java.awt.Image p = i.picture();
        if (p == null) throw new java.io.IOException("the picture could not be read");
        HBackLayer.picture(p, r);
    }
}
