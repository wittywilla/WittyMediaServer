// What every shader gets, once per frame. All vec4, so std140 lays it out the same
// everywhere. The hosts (gl.ts and qt/wvrenderer.cpp) fill it in this order.
WV_UBO uniform Frame {
  vec4 iTimes;   // time, flow (time that runs faster with the music), dt, beats so far
  vec4 iBands;   // bass, mid, treble, level: 0 to 1, each following its own loudness
  vec4 iBeats;   // seconds since the last four beats, newest first
  vec4 iView;    // width, height, width / height, history row
  vec4 iStyle;   // intensity, calm (0 or 1), this effect's seed (0 to 1), beat pulse (0 to 1)
  vec4 iHost;    // flip (1 where a rendered texture reads upside down), clip y, guard's dead band, guard's rise per second
  vec4 iMix;     // transition: progress (0 to 1), style (0 to 3), unused, unused
  vec4 iPalA;    // the colours: a + b * cos(2 pi (c t + d))
  vec4 iPalB;
  vec4 iPalC;
  vec4 iPalD;
};
