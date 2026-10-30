package org.dvb.application;
public interface AppAttributes {
    int DVB_J_application = 1, DVB_HTML_application = 2, BD_J_application = 1;
    int getType();
    String getName();
    String getName(String lang) throws LanguageNotAvailableException;
    String[][] getNames();
    String[] getProfiles();
    int[] getVersions(String p) throws IllegalProfileParameterException;
    boolean getIsServiceBound();
    boolean isStartable();
    AppID getIdentifier();
    int getPriority();
    javax.tv.locator.Locator getServiceLocator();
    Object getProperty(String k);
    boolean isVisible();
}
