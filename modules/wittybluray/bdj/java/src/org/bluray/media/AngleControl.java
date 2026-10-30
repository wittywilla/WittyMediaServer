package org.bluray.media;
public interface AngleControl extends javax.media.Control {
    int getCurrentAngle();
    void selectDefaultAngle();
    void selectAngle(int a) throws InvalidAngleException, org.davic.media.NotAuthorizedException;
    void addAngleChangeListener(AngleChangeListener l);
    void removeAngleChangeListener(AngleChangeListener l);
}
