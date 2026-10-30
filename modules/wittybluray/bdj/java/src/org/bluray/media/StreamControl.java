package org.bluray.media;
/** A selectable stream by number, the shape of the audio and subtitle controls. */
public interface StreamControl extends javax.media.Control {
    int getCurrentStreamNumber();
    void selectStreamNumber(int n) throws StreamNotAvailableException, org.davic.media.NotAuthorizedException;
    int[] listAvailableStreamNumbers();
    String getCurrentLanguage();
    String[] listAvailableLanguages();
    String selectDefaultLanguage() throws org.davic.media.NotAuthorizedException;
    void selectLanguage(String l) throws org.davic.media.LanguageNotAvailableException, org.davic.media.NotAuthorizedException;
}
