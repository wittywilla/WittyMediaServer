package org.havi.ui;
public class HVideoConfigTemplate extends HScreenConfigTemplate {
    public static final int GRAPHICS_MIXING = 15;
    public HVideoConfigTemplate() {}
    public boolean isConfigSupported(HScreenConfiguration c) { return c instanceof HVideoConfiguration; }
}
