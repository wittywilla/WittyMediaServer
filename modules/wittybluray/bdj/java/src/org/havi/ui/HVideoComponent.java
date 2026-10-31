package org.havi.ui;
public abstract class HVideoComponent extends HComponent {
    private final java.util.List<org.havi.ui.event.HScreenLocationModifiedListener> ls = new java.util.concurrent.CopyOnWriteArrayList<org.havi.ui.event.HScreenLocationModifiedListener>();
    protected HVideoComponent() {}
    public HVideoDevice getVideoDevice() { return HScreen.getDefaultHScreen().getDefaultHVideoDevice(); }
    public HScreenPoint getOnScreenLocation() { return new HScreenPoint(getX() / (float) witty.bdj.Host.width, getY() / (float) witty.bdj.Host.height); }
    public void addOnScreenLocationModifiedListener(org.havi.ui.event.HScreenLocationModifiedListener l) { if (l != null) ls.add(l); }
    public void removeOnScreenLocationModifiedListener(org.havi.ui.event.HScreenLocationModifiedListener l) { ls.remove(l); }
}
