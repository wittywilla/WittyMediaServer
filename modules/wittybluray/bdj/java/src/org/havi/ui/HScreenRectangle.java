package org.havi.ui;
public class HScreenRectangle {
    public float x, y, width, height;
    public HScreenRectangle(float x, float y, float w, float h) { this.x = x; this.y = y; width = w; height = h; }
    public void setLocation(float x, float y) { this.x = x; this.y = y; }
    public void setSize(float w, float h) { width = w; height = h; }
}
