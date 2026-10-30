package org.bluray.ti.selection;
public interface TitleContext extends javax.tv.service.selection.ServiceContext {
    void start(org.bluray.ti.Title t, boolean restart) throws SecurityException;
}
