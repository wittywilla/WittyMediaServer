package org.dvb.ui;

import java.awt.AlphaComposite;
import java.awt.Composite;

public final class DVBAlphaComposite implements Composite {
    public static final int CLEAR = 1, SRC = 2, SRC_OVER = 3, DST_OVER = 4, SRC_IN = 5, DST_IN = 6, SRC_OUT = 7, DST_OUT = 8, DST = 9, SRC_ATOP = 10, DST_ATOP = 11, XOR = 12;
    public static final DVBAlphaComposite Clear = new DVBAlphaComposite(CLEAR, 1f), Src = new DVBAlphaComposite(SRC, 1f), SrcOver = new DVBAlphaComposite(SRC_OVER, 1f), DstOver = new DVBAlphaComposite(DST_OVER, 1f),
        SrcIn = new DVBAlphaComposite(SRC_IN, 1f), DstIn = new DVBAlphaComposite(DST_IN, 1f), SrcOut = new DVBAlphaComposite(SRC_OUT, 1f), DstOut = new DVBAlphaComposite(DST_OUT, 1f), Dst = new DVBAlphaComposite(DST, 1f),
        SrcAtop = new DVBAlphaComposite(SRC_ATOP, 1f), DstAtop = new DVBAlphaComposite(DST_ATOP, 1f), Xor = new DVBAlphaComposite(XOR, 1f);
    private final int rule;
    private final float alpha;
    private DVBAlphaComposite(int r, float a) { rule = r; alpha = a; }
    public static DVBAlphaComposite getInstance(int r) { return new DVBAlphaComposite(r, 1f); }
    public static DVBAlphaComposite getInstance(int r, float a) { return new DVBAlphaComposite(r, a); }
    public int getRule() { return rule; }
    public float getAlpha() { return alpha; }
    public java.awt.CompositeContext createContext(java.awt.image.ColorModel s, java.awt.image.ColorModel d, java.awt.RenderingHints h) { return toAWT().createContext(s, d, h); }
    AlphaComposite toAWT() { return AlphaComposite.getInstance(rule, alpha); }
    public boolean equals(Object o) { return o instanceof DVBAlphaComposite && ((DVBAlphaComposite) o).rule == rule && ((DVBAlphaComposite) o).alpha == alpha; }
    public int hashCode() { return rule * 31 + Float.floatToIntBits(alpha); }
}
