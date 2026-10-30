package org.havi.ui;
public interface HLook extends Cloneable {
    void showLook(java.awt.Graphics g, HVisible v, int state);
    void widgetChanged(HVisible v, HChangeData[] c);
    java.awt.Dimension getMinimumSize(HVisible v);
    java.awt.Dimension getPreferredSize(HVisible v);
    java.awt.Dimension getMaximumSize(HVisible v);
    boolean isOpaque(HVisible v);
    java.awt.Insets getInsets(HVisible v);
}
