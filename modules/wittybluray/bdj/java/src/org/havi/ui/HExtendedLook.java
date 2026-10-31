package org.havi.ui;
public interface HExtendedLook extends HLook {
    void fillBackground(java.awt.Graphics g, HVisible v, int state);
    void renderBorders(java.awt.Graphics g, HVisible v, int state);
    void renderVisible(java.awt.Graphics g, HVisible v, int state);
}
