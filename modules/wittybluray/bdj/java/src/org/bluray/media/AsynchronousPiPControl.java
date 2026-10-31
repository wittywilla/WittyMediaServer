package org.bluray.media;
public interface AsynchronousPiPControl extends PiPControl {
    void start();
    void stop();
    void pause();
    void resume();
}
