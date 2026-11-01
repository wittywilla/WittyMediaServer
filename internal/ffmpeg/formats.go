package ffmpeg

import (
	"path/filepath"
	"strings"
)

// FFmpeg picks how to read a file from what the file looks like, whatever it is called, and some
// of what it can pick reads other files. A "film" whose bytes are FFmpeg's list format reads what
// the list names (with a link beside it, anything the server can read), and a text file becomes
// pictures of its words. Whitelisting protocols doesn't help, as these open plain files. So a file
// from a library is only ever read as a format on this list, of containers, sound and pictures.
//
// It comes from every demuxer in the FFmpeg builds the project ships and the one common on Linux,
// less those that follow addresses or read text: concat, hls, dash, imf, sdp, rtp, rtsp, sap,
// webm_dash_manifest, ffmetadata, tty, bin, xbin, adf, idf, data, the svg reader, the script hosts
// (avisynth, vapoursynth), dvdvideo, lavfi and capture devices. Raw formats are left out as no file
// can be recognised as one. FFmpeg looks through everything it knows and only then checks the list,
// so a file that looks most like something else is refused, which is the point.
const libraryFormats = "" +
	"3dostr,4xm,aa,aac,aax,ac3,ac4,ace,acm,act,adp,ads,adx,aea,afc,aiff,aix,alias_pix,alp,amr,amrnb,amrwb,anm,apac," +
	"apc,ape,apm,apng,aptx,aptx_hd,apv,aqtitle,argo_asf,argo_brp,argo_cvg,asf,asf_o,ass,ast,astc,au,av1,avi,avr," +
	"avs,avs2,avs3,bethsoftvid,bfi,bfstm,bink,binka,bit,bmp_pipe,bmv,boa,bonk,brender_pix,brstm,c93,caf,cavsvideo," +
	"cdg,cdxl,cine,codec2,codec2raw,cri_pipe,daud,dcstr,dds_pipe,derf,dfa,dfpwm,dhav,dirac,dnxhd,dpx_pipe,dsf," +
	"dsicin,dss,dts,dtshd,dv,dvbsub,dvbtxt,dxa,ea,ea_cdata,eac3,epaf,evc,exr_pipe,film_cpk,filmstrip,fits,flac," +
	"flic,flv,frm,fsb,fwse,g722,g723_1,g726,g726le,g728,g729,gdv,gem_pipe,genh,gif,gif_pipe,gsm,gxf,h261,h263,h264," +
	"hca,hcom,hdr_pipe,hevc,hnm,hxvs,iamf,ico,idcin,iff,ifv,ilbc,ingenient,ipmovie,ipu,ircam,iss,iv8,ivf,ivr," +
	"j2k_pipe,jacosub,jpeg_mpf,jpeg_pipe,jpegls_pipe,jpegxl_anim,jpegxl_pipe,jpegxs_pipe,jv,ktx,kux,kvag,laf,lc3," +
	"libgme,libopenmpt,live_flv,lmlm4,loas,lrc,luodat,lvf,lxf,m4v,matroska,webm,mca,mcc,mgsts,microdvd,mjpeg," +
	"mjpeg_2000,mlp,mlv,mm,mmf,mods,moflex,mov,mp4,m4a,3gp,3g2,mj2,mp3,mpc,mpc8,mpeg,mpegts,mpegtsraw,mpegvideo," +
	"mpjpeg,mpl2,mpsub,msf,msnwctcp,msp,mtaf,mtv,musx,mv,mvi,mvr,mxf,mxg,nc,nistsphere,nsp,nsv,nut,nuv,obu,ogg,oma," +
	"osq,paf,pam_pipe,pbm_pipe,pcx_pipe,pdv,pfm_pipe,pgm_pipe,pgmyuv_pipe,pgx_pipe,phm_pipe,photocd_pipe," +
	"pictor_pipe,pjs,pmp,png_pipe,pp_bnk,ppm_pipe,psd_pipe,psxstr,pva,pvf,qcp,qdraw_pipe,qoa,qoi_pipe,r3d,rcwt," +
	"realtext,redspark,rka,rl2,rm,roq,rpl,rsd,rso,rw2_pipe,s337m,sami,sbc,sbg,scc,scd,sdns,sdr2,sds,sdx,ser,sga," +
	"sgi_pipe,shn,siff,simbiosis_imx,sln,smjpeg,smk,smush,sol,sox,spdif,srt,stl,subviewer,subviewer1,sunrast_pipe," +
	"sup,svag,svs,swf,tak,tedcaptions,thp,tiertexseq,tiff_pipe,tmv,truehd,tta,txd,ty,usm,vag,vbn_pipe,vc1,vc1test," +
	"vidc,vividas,vivo,vmd,vobsub,voc,vpk,vplayer,vqf,vvc,w64,wady,wav,wavarc,wc3movie,webp_anim,webp_pipe,webvtt," +
	"wsaud,wsd,wsvqa,wtv,wv,wve,xa,xbm_pipe,xmd,xmv,xpm_pipe,xvag,xwd_pipe,xwma,yop,yuv4mpegpipe"

// patternChars make image2 read a file's name as a pattern of many files. Without them it reads the one.
const patternChars = "%*?{["

// libraryFormatsFor is the list for one file: image2 is how FFmpeg reads most pictures, and is
// only safe for a name that can't be taken for a pattern.
func libraryFormatsFor(path string) string {
	if strings.ContainsAny(path, patternChars) {
		return libraryFormats
	}
	return libraryFormats + ",image2"
}

// literalPicture says a file with such a name is a picture of a kind image2 reads, so it is
// told to read that one file and nothing else.
func literalPicture(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".jpe", ".png", ".bmp", ".tif", ".tiff", ".webp", ".gif", ".jxl", ".tga", ".jp2":
		return strings.ContainsAny(path, patternChars)
	}
	return false
}

// discFormats are what a disc's lists hold: program streams on a DVD, transport streams on a Blu-ray.
const discFormats = "concat,mpeg,mpegvideo,mpegts,mpegtsraw"
