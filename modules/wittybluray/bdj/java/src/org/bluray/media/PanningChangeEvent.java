package org.bluray.media;
public class PanningChangeEvent extends java.util.EventObject {
    private final float x, y;
    public PanningChangeEvent(Object s, float leftRight, float frontRear) { super(s); x = leftRight; y = frontRear; }
    public float getLeftRight() { return x; }
    public float getFrontRear() { return y; }
}
