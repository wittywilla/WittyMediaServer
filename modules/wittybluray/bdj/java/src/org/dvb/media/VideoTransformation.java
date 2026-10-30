package org.dvb.media;
public class VideoTransformation {
    private java.awt.Rectangle clip;
    private float xs = 1, ys = 1;
    private org.havi.ui.HScreenPoint at = new org.havi.ui.HScreenPoint(0, 0);
    public VideoTransformation() {}
    public VideoTransformation(java.awt.Rectangle c, float x, float y, org.havi.ui.HScreenPoint p) { clip = c; xs = x; ys = y; at = p; }
    public void setClipRegion(java.awt.Rectangle c) { clip = c; }
    public java.awt.Rectangle getClipRegion() { return clip; }
    public void setScalingFactors(float x, float y) { xs = x; ys = y; }
    public float[] getScalingFactors() { return new float[] {xs, ys}; }
    public void setVideoPosition(org.havi.ui.HScreenPoint p) { at = p; }
    public org.havi.ui.HScreenPoint getVideoPosition() { return at; }
    public boolean isPanAndScan() { return false; }
}
