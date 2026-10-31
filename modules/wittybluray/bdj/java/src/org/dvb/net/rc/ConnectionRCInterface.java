package org.dvb.net.rc;

public class ConnectionRCInterface extends RCInterface {
    protected ConnectionRCInterface() {}
    public void reserve(org.davic.resources.ResourceClient c, Object requestData) throws PermissionDeniedException { throw new PermissionDeniedException("no return channel"); }
    public void release() {}
    public void setTarget(ConnectionParameters p) throws IncompleteTargetException { throw new IncompleteTargetException("no return channel"); }
    public ConnectionParameters getCurrentTarget() { return null; }
    public void connect() throws java.io.IOException, IncompleteTargetException, PermissionDeniedException { throw new PermissionDeniedException("no return channel"); }
    public void disconnect() throws PermissionDeniedException {}
    public void addConnectionListener(ConnectionListener l) {}
    public void removeConnectionListener(ConnectionListener l) {}
}
