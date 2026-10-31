package org.havi.ui;
public abstract class HEmulatedGraphicsDevice extends HGraphicsDevice {
    protected HEmulatedGraphicsDevice() {}
    public boolean setGraphicsConfiguration(HEmulatedGraphicsConfiguration n) throws SecurityException, HPermissionDeniedException, HConfigurationException { return n != null; }
}
