package org.havi.ui;
public class HGraphicsConfigTemplate extends HScreenConfigTemplate {
    public static final int VIDEO_MIXING = 10, MATTE_SUPPORT = 11, IMAGE_SCALING_SUPPORT = 12;
    public HGraphicsConfigTemplate() {}
    public boolean isConfigSupported(HScreenConfiguration c) { return c instanceof HGraphicsConfiguration; }
}
