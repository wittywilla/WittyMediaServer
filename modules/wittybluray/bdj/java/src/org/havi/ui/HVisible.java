package org.havi.ui;

import java.awt.Graphics;
import java.awt.Image;

public class HVisible extends HComponent implements HState {
    public static final int NO_BACKGROUND_FILL = 0, BACKGROUND_FILL = 1;
    public static final int NO_DEFAULT_WIDTH = -1, NO_DEFAULT_HEIGHT = -1, NO_DEFAULT_SIZE = -1;
    public static final int RESIZE_NONE = 0, RESIZE_PRESERVE_ASPECT = 1, RESIZE_ARBITRARY = 2;
    public static final int HALIGN_LEFT = 0, HALIGN_CENTER = 1, HALIGN_RIGHT = 2, HALIGN_JUSTIFY = 3, VALIGN_TOP = 0, VALIGN_CENTER = 4, VALIGN_BOTTOM = 8, VALIGN_JUSTIFY = 12;
    public static final int FIRST_CHANGE = 0, TEXT_CONTENT_CHANGE = 0, GRAPHIC_CONTENT_CHANGE = 1, ANIMATE_CONTENT_CHANGE = 2, CONTENT_CHANGE = 3, STATE_CHANGE = 4, CARET_POSITION_CHANGE = 5, ECHO_CHAR_CHANGE = 6, EDIT_MODE_CHANGE = 7, MIN_MAX_CHANGE = 8, THUMB_OFFSETS_CHANGE = 9, ORIENTATION_CHANGE = 10, TEXT_VALUE_CHANGE = 11, ITEM_VALUE_CHANGE = 12, LIST_CONTENT_CHANGE = 13, LIST_ICONSIZE_CHANGE = 14, LIST_LABELSIZE_CHANGE = 15, LIST_MULTISELECTION_CHANGE = 16, LIST_SCROLLPOSITION_CHANGE = 17, SIZE_CHANGE = 18, BORDER_CHANGE = 19, REPEAT_COUNT_CHANGE = 20, ANIMATION_POSITION_CHANGE = 21, LIST_SELECTION_CHANGE = 22, UNKNOWN_CHANGE = 23, LAST_CHANGE = 23;
    private final Object[] texts = new Object[8], pics = new Object[8], anims = new Object[8], custom = new Object[8];
    private int state = NORMAL_STATE, bg = NO_BACKGROUND_FILL, resize = RESIZE_NONE, align = 0;
    private HLook look = new HDefaultLook();
    private java.awt.Dimension defaultSize;

    public HVisible() { this(new HDefaultLook()); }
    public HVisible(HLook l) { look = l; }
    public HVisible(HLook l, int x, int y, int w, int h) { super(x, y, w, h); look = l; }
    private static int at(int s) { return (s - FIRST_STATE) & 7; }
    public void setTextContent(String t, int mask) { each(texts, t, mask); }
    public void setGraphicContent(Image i, int mask) { each(pics, i, mask); }
    public void setAnimateContent(Image[] a, int mask) { each(anims, a, mask); }
    public void setContent(Object o, int mask) { each(custom, o, mask); }
    private void each(Object[] slot, Object v, int mask) {
        if (mask == ALL_STATES) { for (int i = 0; i < 8; i++) slot[i] = v; }
        else slot[at(mask)] = v;
        repaint();
    }
    public String getTextContent(int s) { return (String) (texts[at(s)] != null ? texts[at(s)] : texts[0]); }
    public Image getGraphicContent(int s) { return (Image) (pics[at(s)] != null ? pics[at(s)] : pics[0]); }
    public Image[] getAnimateContent(int s) { return (Image[]) (anims[at(s)] != null ? anims[at(s)] : anims[0]); }
    public Object getContent(int s) { return custom[at(s)]; }
    public void setInteractionState(int s) { state = s; repaint(); }
    public int getInteractionState() { return state; }
    public void setLook(HLook l) { look = l; repaint(); }
    public HLook getLook() { return look; }
    public void setBackgroundMode(int m) { bg = m; repaint(); }
    public int getBackgroundMode() { return bg; }
    public void setResizeMode(int m) { resize = m; }
    public int getResizeMode() { return resize; }
    public void setHorizontalAlignment(int a) { align = a; }
    public void setVerticalAlignment(int a) { align = a; }
    public int getHorizontalAlignment() { return align & 3; }
    public int getVerticalAlignment() { return align & 12; }
    public void setDefaultSize(java.awt.Dimension d) { defaultSize = d; }
    public java.awt.Dimension getDefaultSize() { return defaultSize; }
    public boolean isOpaque() { return bg == BACKGROUND_FILL; }
    public void paint(Graphics g) { if (look != null) look.showLook(g, this, state); }
    public void update(Graphics g) { paint(g); }
}
