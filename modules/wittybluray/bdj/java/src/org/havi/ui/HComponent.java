package org.havi.ui;

import java.awt.Component;
import java.awt.event.KeyEvent;

public abstract class HComponent extends Component implements HMatteLayer, org.dvb.ui.TestOpacity {
    private HMatte matte;
    public HComponent() {}
    public HComponent(int x, int y, int w, int h) { setBounds(x, y, w, h); }
    public void setMatte(HMatte m) { matte = m; }
    public HMatte getMatte() { return matte; }
    public boolean isOpaque() { return false; }
    public boolean isDoubleBuffered() { return false; }
    public void setEnabled(boolean e) { super.setEnabled(e); }
    /** The arrow keys, for components that say who their neighbours are. */
    public static void navigate(Component c, int code, int id) {
        if (id != KeyEvent.KEY_PRESSED || !(c instanceof HNavigable)) return;
        HNavigable n = (HNavigable) c;
        HNavigationInputPreferred next = null;
        switch (code) {
            case KeyEvent.VK_UP: next = n.getMove(code); break;
            case KeyEvent.VK_DOWN: next = n.getMove(code); break;
            case KeyEvent.VK_LEFT: next = n.getMove(code); break;
            case KeyEvent.VK_RIGHT: next = n.getMove(code); break;
            default: return;
        }
        if (next instanceof Component) witty.bdj.Keys.setFocus((Component) next);
    }
}
