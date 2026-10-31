package org.havi.ui;
public class HEmulatedGraphicsConfiguration extends HGraphicsConfiguration {
    protected HEmulatedGraphicsConfiguration() {}
    public HGraphicsConfiguration getImplementation() { return HScreen.getDefaultHScreen().getDefaultHGraphicsDevice().getCurrentConfiguration(); }
    public HGraphicsConfigTemplate getConfigTemplate() { return new HEmulatedGraphicsConfigTemplate(); }
}
