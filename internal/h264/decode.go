package h264

import (
	/*
		#cgo CFLAGS : -I/usr/include/
		#cgo LDFLAGS: -L/usr/lib64/ -lavcodec -lavformat -lavutil -lx264 -lva -lrt -lpthread -ldl -lm

		#include <libavcodec/avcodec.h>
		#include <libavformat/avformat.h>
		#include <libavutil/avutil.h>
		#include <libavutil/imgutils.h>
		#include <libavutil/samplefmt.h>
		#include <stdio.h>
		#include <string.h>
		#include <time.h>
		#include <stdarg.h>

		void logerror(const char* fmt, ...) {
			char buff[1024];
			struct tm *sTm;

			time_t now = time (0);
			sTm = localtime(&now);

			strftime (buff, sizeof(buff), "%Y-%m-%d %H:%M:%S", sTm);
			strcat(buff, " Error [H264 DECODE] ");
			strcat(buff, fmt);

			va_list argptr;
			va_start(argptr, fmt);
			vfprintf(stdout, buff, argptr);
			va_end(argptr);
		}

		void loginfo(const char* fmt, ...) {
			char buff[1024];
			struct tm *sTm;

			time_t now = time (0);
			sTm = localtime(&now);

			strftime (buff, sizeof(buff), "%Y-%m-%d %H:%M:%S", sTm);
			strcat(buff, " Info [H264 DECODE] ");
			strcat(buff, fmt);

			va_list argptr;
			va_start(argptr, fmt);
			vfprintf(stdout, buff, argptr);
			va_end(argptr);
		}

		typedef struct {
			AVFormatContext *fmtCtx;
			AVCodec         *c;
			AVCodecContext  *ctx;
			AVFrame         *f;
			uint8_t         *video_dst_data[4];
			int             video_dst_linesize[4];
			int             video_dst_bufsize;
			char            pathname[256];
		} h264dec_t ;

		static int h264dec_new(h264dec_t *h, const char *probeFN, const char *pathN) {
			int ret, stream_index;
			AVStream *st;

			strcpy(h->pathname, pathN);
			h->fmtCtx = avformat_alloc_context();
			ret = avformat_open_input(&(h->fmtCtx), probeFN, NULL, NULL);
			if (ret < 0) {
				logerror("path: %s - Error of func avformat_open_input: %s\n", h->pathname, av_err2str(ret));
				return ret;
			}

			ret = avformat_find_stream_info(h->fmtCtx, NULL);
			if (ret < 0) {
				logerror("path: %s - Error of func avformat_find_stream_info: %s\n", h->pathname, av_err2str(ret));
				return ret;
			}

			ret = av_find_best_stream(h->fmtCtx, AVMEDIA_TYPE_VIDEO, -1, -1, NULL, 0);
			if (ret < 0) {
				logerror("path: %s - could not find video stream in probe file '%s'\n", h->pathname, probeFN);
				return ret;
			}

			stream_index = ret;
			st = h->fmtCtx->streams[stream_index];
			if (!(h->c = avcodec_find_decoder(st->codecpar->codec_id))) {
				logerror("path: %s - Codec of %s not found\n", h->pathname, avcodec_get_name(st->codecpar->codec_id));
				return -1;
			}

			if (!(h->ctx = avcodec_alloc_context3(h->c))) {
				logerror("path: %s - could not allocate video codec context\n", h->pathname);
				return -1;
			}

			ret = avcodec_parameters_to_context(h->ctx, st->codecpar);
			if (ret < 0) {
				logerror("path: %s - failed to copy codec parameters to decoder context\n", h->pathname);
				return ret;
			}

			h->f = av_frame_alloc();
			//h->ctx->debug = 0x3;

			ret = avcodec_open2(h->ctx, h->c, 0);
			if (ret < 0) {
				logerror("path: %s - error of func avcodec_open2: %s\n", h->pathname, av_err2str(ret));
				return ret;
			}

			loginfo("path: %s - a decoder of %s created: width=%d, height=%d, pix_fmt=%s\n", \
					h->pathname, avcodec_get_name(st->codecpar->codec_id), \
					h->ctx->width, h->ctx->height, av_get_pix_fmt_name(h->ctx->pix_fmt));

			//allocate image where the decoded image will be put
			ret = av_image_alloc(h->video_dst_data, h->video_dst_linesize,
					h->ctx->width, h->ctx->height, h->ctx->pix_fmt, 1);
			if (ret < 0) {
				logerror("path: %s - av_image_alloc() could not allocate buffer: %s\n", h->pathname, av_err2str(ret));
				return ret;
			}
			h->video_dst_bufsize = ret;

			return 0;
		}

		static int h264dec_decode(h264dec_t *h, uint8_t *data, int len) {
			int ret;
			AVPacket pkt;

			av_init_packet(&pkt);
			av_packet_from_data(&pkt, data, len);

			ret = avcodec_send_packet(h->ctx, &pkt);
			if (ret < 0) {
				logerror("path: %s - Error of avcodec_send_packet(): %s\n", h->pathname, av_err2str(ret));
				return ret;
			}

			//only one idr, so no need loop
			av_frame_unref(h->f);
			ret = avcodec_receive_frame(h->ctx, h->f);
			if (ret == AVERROR(EAGAIN) || ret == AVERROR_EOF) {
				return 1;
			} else if (ret < 0) {
				logerror("path: %s - Error of avcodec_receive_frame(): %s\n", h->pathname, av_err2str(ret));
				return ret;
			}

			//frame alignment
			av_image_copy(h->video_dst_data, h->video_dst_linesize,
					(const uint8_t **)(h->f->data), h->f->linesize,
					h->ctx->pix_fmt, h->ctx->width, h->ctx->height);

			return 0;
		}

		static void libav_init() {
			//av_log_set_level(AV_LOG_DEBUG);
		}
	*/
	"C"
)
import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"time"
	"unsafe"
)

const (
	PROBEPATH         = `/dev/shm/avformatprobe/`
	SNAPONEPERSECOND  = 10 * time.Second
	RETRYLIMIT        = 10
	RETRPAUSEDURITION = 1 * time.Hour
	IMGFILEPREFIX     = `TPC`
	BASHTIMEOUT       = SNAPONEPERSECOND / 2
	ERRTAG            = `Error [H264 DECODE]`
	WARNTAG           = `Warning [H264 DECODE]`
	INFOTAG           = `Info [H264 DECODE]`

	OUTIMGPATH = `/dev/shm`    //this filepath is for saving output pictures
	SNAP_ALL_IDR = true

)

func init() {
	C.libav_init()
}

type H264Decoder struct {
	m      C.h264dec_t
	buffer []byte
	//startPTS      time.Time
	ctx           context.Context
	gotNewData    chan struct{}
	pathName      string
	probeFileName string
	inited        bool //video codec with AVCodecContext ready or not
	inService     bool //after RETRYLIMIT continuous failures to decode a frame,
	errChan       chan error
	contRetries   int
}

func NewH264Decoder(ctx context.Context, pathName string, header []byte) (m *H264Decoder, err error) {
	m = &H264Decoder{}

	m.pathName = pathName
	m.gotNewData = make(chan struct{}, 10)
	m.buffer = []byte{}
	m.errChan = make(chan error, 10)
	m.ctx = ctx
	m.inService = true

	_ = os.MkdirAll(PROBEPATH, 0775)
	m.probeFileName = PROBEPATH + pathName

	go func(ctx context.Context) {
		ticker := time.NewTicker(RETRPAUSEDURITION)
		select {
		case <-ticker.C:
			m.inService = true
			m.contRetries = 0
		case <-ctx.Done():
			ticker.Stop()
			return
		}
	}(ctx)

	go func(ctx context.Context) {
		for {
			if m.contRetries >= RETRYLIMIT {
				return //exit goroutine and terminate snap pictures on the path
			}

			select {
			case <-m.gotNewData:
				if !m.inited {
					if err := m.initVideoCodec(); err != nil {
						log.Println(ERRTAG, "path:", m.pathName, "-", err)
						m.incRetry()
					} else {
						m.inited = true
					}
				}

				m.decodeToJpg()
			case <-ctx.Done():
				return
			}
		}
	}(ctx)

	return
}

func (m *H264Decoder) IsInited() bool {
	return m.inited
}

func (m *H264Decoder) InService() bool {
	return m.inService
}

func (m *H264Decoder) incRetry() {
	m.contRetries += 1
	if m.contRetries >= RETRYLIMIT {
		m.inService = false
	}
}

func (m *H264Decoder) initVideoCodec() error {
	if err := m.writeProbeFile(); err != nil {
		return err
	}

	cstrProbeFN := C.CString(m.probeFileName)
	defer C.free(unsafe.Pointer(cstrProbeFN))

	r := C.h264dec_new(&m.m, cstrProbeFN, C.CString(m.pathName))
	if int(r) < 0 {
		return errors.New(`failed on C.h264dec_new()`)
	}

	return nil
}

func (m *H264Decoder) decodeToJpg() {
	defer m.clearBuffer()

	if m.inited {
		if err := m.intraDecode(); err != nil {
			log.Println(ERRTAG, "path:", m.pathName, "-", err)
			m.incRetry()
		} else {
			m.contRetries = 0
			return
		}
	}

	//extraDecode() use ffmpeg in $PATH to do final try if intraDecode() failed
	if err := m.extraDecode(); err != nil {
		log.Println(ERRTAG, "path:", m.pathName, "-",  err)
		m.incRetry()
	} else {
		m.contRetries = 0
	}
}

func (m *H264Decoder) intraDecode() error {
	r := C.h264dec_decode(
		&m.m,
		(*C.uint8_t)(unsafe.Pointer(&m.buffer[0])),
		(C.int)(len(m.buffer)),
	)

	if int(r) < 0 {
		return errors.New("h264 video frame decode failed")
	}

	//(ret == AVERROR(EAGAIN) || ret == AVERROR_EOF)
	if int(r) == 1 {
		return nil
	}

	w := int(m.m.f.width)
	h := int(m.m.f.height)
	frame := C.GoBytes(unsafe.Pointer(m.m.video_dst_data[0]), C.int(m.m.video_dst_bufsize))
	yuv, err := getYuvFromI420(frame, w, h)
	if err != nil {
		return err
	}

	jpgFN := m.jpgFileName()
	f, err := os.Create(jpgFN)
	if err != nil {
		return err
	}
	defer f.Close()

	if err = jpeg.Encode(f, yuv, nil); err != nil {
		return err
	}

	log.Println(INFOTAG, "path:", m.pathName, "- intraDecode() snap a picture",  jpgFN)

	return nil
}

func (m *H264Decoder) extraDecode() error {
	if err := m.writeProbeFile(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(m.ctx, BASHTIMEOUT)
	defer cancel()

	jpgFN := m.jpgFileName()
	script := fmt.Sprintf(`ffmpeg -i %s -y -ss 00:00:00 -vframes 1 %s &>/dev/null`,
		m.probeFileName, jpgFN)
	cmd := exec.CommandContext(ctx, `bash`, `-c`, script)

	err := cmd.Run()
	if err != nil {
		return err
	}

	log.Println(INFOTAG, "path:", m.pathName, "- intraDecode() snap a picture",  jpgFN)
	return nil
}

func (m *H264Decoder) writeProbeFile() error {
	return ioutil.WriteFile(m.probeFileName, m.buffer, 0664)
}

func (m *H264Decoder) delProbeFile() error {
	return os.Remove(m.probeFileName)
}

func (m *H264Decoder) GatherData(data []byte) {
	m.buffer = data
	m.gotNewData <- struct{}{}
}

func (m *H264Decoder) clearBuffer() {
	m.buffer = []byte{}
}

func (m *H264Decoder) jpgFileName() string {
	imgName := OUTIMGPATH + `/` + IMGFILEPREFIX + strconv.FormatInt(time.Now().Unix(), 10) +
		`.` + m.pathName + `.jpg`
	return imgName
}

func fromCPtr(buf unsafe.Pointer, size int) (ret []uint8) {
	hdr := (*reflect.SliceHeader)((unsafe.Pointer(&ret)))
	hdr.Cap = size
	hdr.Len = size
	hdr.Data = uintptr(buf)
	return
}

func getYuvFromI420(frame []byte, width, height int) (*image.YCbCr, error) {
	yi := width * height
	cbi := yi + width*height/4
	cri := cbi + width*height/4

	if cri > len(frame) {
		return nil, fmt.Errorf("frame length (%d) less than expected (%d)", len(frame), cri)
	}

	return &image.YCbCr{
		Y:              frame[:yi],
		YStride:        width,
		Cb:             frame[yi:cbi],
		Cr:             frame[cbi:cri],
		CStride:        width / 2,
		SubsampleRatio: image.YCbCrSubsampleRatio420,
		Rect:           image.Rect(0, 0, width, height),
	}, nil
}
