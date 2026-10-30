package org.havi.ui;
import java.awt.Component;
public interface HComponentOrdering {
    Component addBefore(Component c, Component b);
    Component addAfter(Component c, Component b);
    boolean popToFront(Component c);
    boolean pushToBack(Component c);
    boolean pop(Component c);
    boolean push(Component c);
    boolean popInFrontOf(Component m, Component n);
    boolean pushBehind(Component m, Component n);
}
