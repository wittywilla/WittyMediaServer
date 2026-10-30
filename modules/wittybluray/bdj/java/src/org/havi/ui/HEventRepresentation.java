package org.havi.ui;
public class HEventRepresentation {
    public static final int ER_TYPE_NOT_SUPPORTED = 0, ER_TYPE_STRING = 1, ER_TYPE_COLOR = 2, ER_TYPE_SYMBOL = 4;
    public boolean isSupported() { return true; }
    public int getType() { return ER_TYPE_STRING; }
    public java.awt.Color getColor() { return null; }
    public String getString() { return ""; }
    public java.awt.Image getSymbol() { return null; }
}
