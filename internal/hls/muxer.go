package hls

import (
	"context"
	"io"
	"time"

	"github.com/aler9/gortsplib"

	"github.com/aler9/rtsp-simple-server/internal/h264"
)

const (
	// an offset is needed to
	// - avoid negative PTS values
	// - avoid PTS < DTS during startup
	ptsOffset = 2 * time.Second

	segmentMinAUCount = 100
)

// Muxer is a HLS muxer.
type Muxer struct {
	hlsSegmentCount    int
	hlsSegmentDuration time.Duration
	videoTrack         *gortsplib.Track
	audioTrack         *gortsplib.Track

	h264Conf        *gortsplib.TrackConfigH264
	aacConf         *gortsplib.TrackConfigAAC
	videoDTSEst     *h264.DTSEstimator
	audioAUCount    int
	currentSegment  *segment
	startPCR        time.Time
	startPTS        time.Duration
	primaryPlaylist *primaryPlaylist
	streamPlaylist  *streamPlaylist

	pathName      string
	h264Decoder   *h264.H264Decoder
	cancelH264Dcd context.CancelFunc
	cancelTicker  context.CancelFunc
	snapSignal    bool
}

// NewMuxer allocates a Muxer.
func NewMuxer(
	hlsSegmentCount int,
	hlsSegmentDuration time.Duration,
	videoTrack *gortsplib.Track,
	audioTrack *gortsplib.Track,
	pathName string) (*Muxer, error) {
	var h264Conf *gortsplib.TrackConfigH264
	if videoTrack != nil {
		var err error
		h264Conf, err = videoTrack.ExtractConfigH264()
		if err != nil {
			return nil, err
		}
	}

	var aacConf *gortsplib.TrackConfigAAC
	if audioTrack != nil {
		var err error
		aacConf, err = audioTrack.ExtractConfigAAC()
		if err != nil {
			return nil, err
		}
	}

	var avCtxExtradata [][]byte
	avCtxExtradata = append(avCtxExtradata, h264Conf.SPS)
	avCtxExtradata = append(avCtxExtradata, h264Conf.PPS)
	encAvCtxExtradata, err := h264.EncodeAnnexB(avCtxExtradata)
	if err != nil {
		return nil, err
	}

	ctxH264Dcd, cancelH264Dcd := context.WithCancel(context.Background())
	h264Dec, err := h264.NewH264Decoder(ctxH264Dcd, pathName, encAvCtxExtradata)
	if err != nil {
		cancelH264Dcd()
		return nil, err
	}

	m := &Muxer{
		hlsSegmentCount:    hlsSegmentCount,
		hlsSegmentDuration: hlsSegmentDuration,
		videoTrack:         videoTrack,
		audioTrack:         audioTrack,
		h264Conf:           h264Conf,
		aacConf:            aacConf,
		videoDTSEst:        h264.NewDTSEstimator(),
		currentSegment:     newSegment(videoTrack, audioTrack, h264Conf, aacConf),
		primaryPlaylist:    newPrimaryPlaylist(videoTrack, audioTrack, h264Conf),
		streamPlaylist:     newStreamPlaylist(hlsSegmentCount),
		pathName:           pathName,
		h264Decoder:        h264Dec,
		cancelH264Dcd:      cancelH264Dcd,
		snapSignal:         true,
	}

	ctxTicker, cancelTicker := context.WithCancel(context.Background())
	ticker := time.NewTicker(h264.SNAPONEPERSECOND)
	go func() {
		for {
			select {
			case <-ticker.C:
				m.snapSignal = true
			case <-ctxTicker.Done():
				ticker.Stop()
				return
			}
		}
	}()
	m.cancelTicker = cancelTicker

	return m, nil
}

// Close closes a Muxer.
func (m *Muxer) Close() {
	m.streamPlaylist.close()
	m.cancelH264Dcd()
	m.cancelTicker()
}

// WriteH264 writes H264 NALUs, grouped by PTS, into the muxer.
func (m *Muxer) WriteH264(pts time.Duration, nalus [][]byte) error {
	/*
		idrPresent := func() bool {
			for _, nalu := range nalus {
				typ := h264.NALUType(nalu[0] & 0x1F)
				if typ == h264.NALUTypeIDR {
					return true
				}
			}
			return false
		}()
	*/
	idrPresent := false
	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)
		if typ == h264.NALUTypeIDR {
			idrPresent = true
		}

		if m.h264Decoder.InService() {
			if (h264.SNAP_ALL_IDR || m.snapSignal) && (typ == h264.NALUTypeIDR) {
				imgNalus := [][]byte{}
				if !m.h264Decoder.IsInited() {
					imgNalus = append(imgNalus, m.h264Conf.SPS)
					imgNalus = append(imgNalus, m.h264Conf.PPS)
				}
				imgNalus = append(imgNalus, nalu)

				encImgNalus, err := h264.EncodeAnnexB(imgNalus)
				if err != nil {
					return err
				}
				m.h264Decoder.GatherData(encImgNalus)
				m.snapSignal = false
			}
		}
	}

	// skip group silently until we find one with a IDR
	if !m.currentSegment.firstPacketWritten && !idrPresent {
		return nil
	}

	if m.currentSegment.firstPacketWritten {
		if idrPresent &&
			m.currentSegment.duration() >= m.hlsSegmentDuration {
			m.streamPlaylist.pushSegment(m.currentSegment)

			m.currentSegment = newSegment(m.videoTrack, m.audioTrack, m.h264Conf, m.aacConf)
			m.currentSegment.setStartPCR(m.startPCR)
		}
	} else {
		m.startPCR = time.Now()
		m.startPTS = pts
		m.currentSegment.setStartPCR(m.startPCR)
	}

	pts = pts + ptsOffset - m.startPTS

	err := m.currentSegment.writeH264(
		m.videoDTSEst.Feed(pts),
		pts,
		idrPresent,
		nalus)
	if err != nil {
		return err
	}

	return nil
}

// WriteAAC writes AAC AUs, grouped by PTS, into the muxer.
func (m *Muxer) WriteAAC(pts time.Duration, aus [][]byte) error {
	if m.videoTrack == nil {
		if m.currentSegment.firstPacketWritten {
			if m.audioAUCount >= segmentMinAUCount &&
				m.currentSegment.duration() >= m.hlsSegmentDuration {
				m.audioAUCount = 0

				m.streamPlaylist.pushSegment(m.currentSegment)

				m.currentSegment = newSegment(m.videoTrack, m.audioTrack, m.h264Conf, m.aacConf)
				m.currentSegment.setStartPCR(m.startPCR)
			}
		} else {
			m.startPCR = time.Now()
			m.startPTS = pts
			m.currentSegment.setStartPCR(m.startPCR)
		}
	} else {
		if !m.currentSegment.firstPacketWritten {
			return nil
		}
	}

	pts = pts + ptsOffset - m.startPTS

	for i, au := range aus {
		auPTS := pts + time.Duration(i)*1000*time.Second/time.Duration(m.aacConf.SampleRate)

		err := m.currentSegment.writeAAC(auPTS, au)
		if err != nil {
			return err
		}

		m.audioAUCount++
	}

	return nil
}

// PrimaryPlaylist returns a reader to read the primary playlist
func (m *Muxer) PrimaryPlaylist() io.Reader {
	return m.primaryPlaylist.reader()
}

// StreamPlaylist returns a reader to read the stream playlist.
func (m *Muxer) StreamPlaylist() io.Reader {
	return m.streamPlaylist.reader()
}

// Segment returns a reader to read a segment.
func (m *Muxer) Segment(fname string) io.Reader {
	return m.streamPlaylist.segment(fname)
}
