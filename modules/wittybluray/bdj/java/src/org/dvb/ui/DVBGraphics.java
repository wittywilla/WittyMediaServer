package org.dvb.ui;

import java.awt.Graphics2D;

public abstract class DVBGraphics extends Graphics2D {
    protected DVBGraphics() {}
    public abstract int[] getAvailableCompositeRules();
    public abstract DVBAlphaComposite getDVBComposite();
    public abstract void setDVBComposite(DVBAlphaComposite c) throws UnsupportedDrawingOperationException;
    public abstract int getType();
    public abstract java.awt.Color getBestColorMatch(java.awt.Color c);
}
