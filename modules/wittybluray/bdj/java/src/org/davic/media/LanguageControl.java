package org.davic.media;
public interface LanguageControl extends javax.media.Control {
    String[] listAvailableLanguages();
    void selectLanguage(String l) throws LanguageNotAvailableException, NotAuthorizedException;
    String getCurrentLanguage();
    String selectDefaultLanguage() throws NotAuthorizedException;
}
