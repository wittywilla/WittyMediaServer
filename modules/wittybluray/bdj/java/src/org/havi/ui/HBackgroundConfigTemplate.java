package org.havi.ui;
public class HBackgroundConfigTemplate extends HScreenConfigTemplate {
    public static final int CHANGEABLE_SINGLE_COLOR = 15, STILL_IMAGE = 16;
    public HBackgroundConfigTemplate() {}
    public boolean isConfigSupported(HScreenConfiguration c) { return c instanceof HBackgroundConfiguration; }
}
