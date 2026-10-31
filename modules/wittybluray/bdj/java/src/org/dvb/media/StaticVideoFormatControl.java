package org.dvb.media;

/** What a player answers for the video format: 16:9 full frame, no conversion in the way. */
public class StaticVideoFormatControl implements VideoFormatControl {
    public int getAspectRatio() { return ASPECT_RATIO_16_9; }
    public int getActiveFormatDefinition() { return AFD_NOT_PRESENT; }
    public int getDisplayAspectRatio() { return ASPECT_RATIO_16_9; }
    public int getDecoderFormatConversion() { return DFC_PROCESSING_NONE; }
    public int getVideoDisplayFormat() { return DFC_PROCESSING_NONE; }
    public void addVideoFormatListener(VideoFormatListener l) {}
    public void removeVideoFormatListener(VideoFormatListener l) {}
    public java.awt.Component getControlComponent() { return null; }
}
