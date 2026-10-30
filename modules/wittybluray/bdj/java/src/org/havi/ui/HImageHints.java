package org.havi.ui;
public class HImageHints {
    public static final int NATURAL_IMAGE = 1, CARTOON = 2, GRAPHICS = 3, LINE_ART = 4;
    private int t = NATURAL_IMAGE;
    public void setType(int v) { t = v; }
    public int getType() { return t; }
}
