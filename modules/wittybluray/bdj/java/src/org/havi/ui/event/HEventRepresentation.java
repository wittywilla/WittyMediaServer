package org.havi.ui.event;
public class HEventRepresentation {
    public static final int ER_TYPE_NOT_SUPPORTED = 0, ER_TYPE_STRING = 1, ER_TYPE_COLOR = 2, ER_TYPE_SYMBOL = 4;
    private int type = ER_TYPE_NOT_SUPPORTED;
    private java.awt.Color color;
    private String text;
    private java.awt.Image symbol;
    protected HEventRepresentation() {}
    HEventRepresentation(int code) {
        switch (code) {
            case HRcEvent.VK_COLORED_KEY_0: color = java.awt.Color.red; break;
            case HRcEvent.VK_COLORED_KEY_1: color = java.awt.Color.green; break;
            case HRcEvent.VK_COLORED_KEY_2: color = java.awt.Color.yellow; break;
            case HRcEvent.VK_COLORED_KEY_3: color = java.awt.Color.blue; break;
            default: text = code >= '0' && code <= '9' ? String.valueOf((char) code) : "Key " + code;
        }
        type = color != null ? ER_TYPE_COLOR : ER_TYPE_STRING;
    }
    protected void setType(int t) { type = t; }
    protected void setColor(java.awt.Color c) { color = c; }
    protected void setString(String s) { text = s; }
    protected void setSymbol(java.awt.Image i) { symbol = i; }
    public boolean isSupported() { return type != ER_TYPE_NOT_SUPPORTED; }
    public int getType() { return type; }
    public java.awt.Color getColor() { return color; }
    public String getString() { return text; }
    public java.awt.Image getSymbol() { return symbol; }
}
