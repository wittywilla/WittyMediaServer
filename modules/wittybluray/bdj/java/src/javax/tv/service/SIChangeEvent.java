package javax.tv.service;
public abstract class SIChangeEvent extends java.util.EventObject {
    private final SIChangeType type;
    private final SIElement element;
    public SIChangeEvent(Object source, SIChangeType t, SIElement e) { super(source); type = t; element = e; }
    public SIChangeType getChangeType() { return type; }
    public SIElement getSIElement() { return element; }
}
