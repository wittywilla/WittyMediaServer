package org.havi.ui;
public class HListElement {
    private String label;
    private java.awt.Image icon;
    public HListElement(String l) { label = l; }
    public HListElement(java.awt.Image i, String l) { icon = i; label = l; }
    public String getLabel() { return label; }
    public java.awt.Image getIcon() { return icon; }
    public void setLabel(String l) { label = l; }
    public void setIcon(java.awt.Image i) { icon = i; }
}
