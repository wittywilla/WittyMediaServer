package org.dvb.media;
public interface VideoFormatControl extends javax.media.Control {
    int AFD_NOT_PRESENT = -1, AFD_16_9_TOP = 2, AFD_14_9_TOP = 3, AFD_GT_16_9 = 4, AFD_SAME = 8, AFD_4_3 = 9, AFD_16_9 = 10, AFD_14_9 = 11, AFD_4_3_SP_14_9 = 13, AFD_16_9_SP_14_9 = 14, AFD_16_9_SP_4_3 = 15;
    int ASPECT_RATIO_UNKNOWN = -1, ASPECT_RATIO_4_3 = 0, ASPECT_RATIO_16_9 = 1, ASPECT_RATIO_2_21_1 = 2;
    int DFC_PROCESSING_UNKNOWN = -1, DFC_PROCESSING_NONE = 0, DFC_PROCESSING_FULL = 1, DFC_PROCESSING_LB_16_9 = 2, DFC_PROCESSING_LB_14_9 = 3, DFC_PROCESSING_CCO = 4, DFC_PROCESSING_PAN_SCAN = 5,
        DFC_PROCESSING_LB_2_21_1_ON_4_3 = 6, DFC_PROCESSING_LB_2_21_1_ON_16_9 = 7, DFC_PLATFORM = 8, DFC_PROCESSING_16_9_ZOOM = 9, DFC_PROCESSING_PILLARBOX_4_3 = 10, DFC_PROCESSING_WIDE_4_3 = 11;
    int getAspectRatio();
    int getActiveFormatDefinition();
    int getDisplayAspectRatio();
    int getDecoderFormatConversion();
    int getVideoDisplayFormat();
    void addVideoFormatListener(VideoFormatListener l);
    void removeVideoFormatListener(VideoFormatListener l);
}
