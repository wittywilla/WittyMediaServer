package org.davic.resources;
public interface ResourceServer {
    void addResourceStatusEventListener(ResourceStatusListener l);
    void removeResourceStatusEventListener(ResourceStatusListener l);
}
