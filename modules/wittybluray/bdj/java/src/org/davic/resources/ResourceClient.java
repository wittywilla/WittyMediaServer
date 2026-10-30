package org.davic.resources;
public interface ResourceClient {
    boolean requestRelease(ResourceProxy p, Object d);
    void release(ResourceProxy p);
    void notifyRelease(ResourceProxy p);
}
