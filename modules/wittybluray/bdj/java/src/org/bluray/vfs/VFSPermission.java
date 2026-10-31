package org.bluray.vfs;
public class VFSPermission extends java.security.BasicPermission {
    public VFSPermission(String name) { super(name); }
    public VFSPermission(String name, String actions) { super(name, actions); }
}
