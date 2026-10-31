package org.davic.mpeg;
/** A stream of a transport stream; a disc has none of these to look at. */
public class ElementaryStream {
    private final int pid, tag;
    protected ElementaryStream(int pid, int tag) { this.pid = pid; this.tag = tag; }
    public int getPID() { return pid; }
    public int getAssociatedTag() { return tag; }
    public int getElementaryStreamType() { return 0; }
}
