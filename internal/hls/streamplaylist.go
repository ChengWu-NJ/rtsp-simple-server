package hls

import (
	"bytes"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"
)

type readerFunc struct {
	wrapped func() []byte
	reader  *bytes.Reader
}

func (r *readerFunc) Read(buf []byte) (int, error) {
	if r.reader == nil {
		cnt := r.wrapped()
		r.reader = bytes.NewReader(cnt)
	}
	return r.reader.Read(buf)
}

type streamPlaylist struct {
	hlsSegmentCount int

	mutex              sync.Mutex
	cond               *sync.Cond
	closed             bool
	segments           []*segment
	segmentByName      map[string]*segment
	segmentDeleteCount int
}

func newStreamPlaylist(hlsSegmentCount int) *streamPlaylist {
	p := &streamPlaylist{
		hlsSegmentCount: hlsSegmentCount,
		segmentByName:   make(map[string]*segment),
	}
	p.cond = sync.NewCond(&p.mutex)
	return p
}

func (p *streamPlaylist) close() {
	func() {
		p.mutex.Lock()
		defer p.mutex.Unlock()
		p.closed = true
	}()

	p.cond.Broadcast()
}

func (p *streamPlaylist) reader() io.Reader {
	return &readerFunc{wrapped: func() []byte {
		p.mutex.Lock()
		defer p.mutex.Unlock()

		if !p.closed && len(p.segments) == 0 {
			p.cond.Wait()
		}

		if p.closed {
			return nil
		}

		cnt := "#EXTM3U\n"
		cnt += "#EXT-X-VERSION:3\n"
		cnt += "#EXT-X-ALLOW-CACHE:NO\n"

		targetDuration := func() uint {
			ret := uint(0)

			// EXTINF, when rounded to the nearest integer, must be <= EXT-X-TARGETDURATION
			for _, f := range p.segments {
				v2 := uint(math.Round(f.duration().Seconds()))
				if v2 > ret {
					ret = v2
				}
			}

			return ret
		}()
		cnt += "#EXT-X-TARGETDURATION:" + strconv.FormatUint(uint64(targetDuration), 10) + "\n"

		cnt += "#EXT-X-MEDIA-SEQUENCE:" + strconv.FormatInt(int64(p.segmentDeleteCount), 10) + "\n"

		for _, f := range p.segments {
			cnt += "#EXTINF:" + strconv.FormatFloat(f.duration().Seconds(), 'f', -1, 64) + ",\n"
			cnt += f.name + ".ts\n"
		}

		return []byte(cnt)
	}}
}

func (p *streamPlaylist) segment(fname string) io.Reader {
	base := strings.TrimSuffix(fname, ".ts")

	p.mutex.Lock()
	f, ok := p.segmentByName[base]
	p.mutex.Unlock()

	if !ok {
		return nil
	}

	return f.reader()
}

func (p *streamPlaylist) pushSegment(t *segment) {
	func() {
		p.mutex.Lock()
		defer p.mutex.Unlock()

		p.segmentByName[t.name] = t
		p.segments = append(p.segments, t)

		if len(p.segments) > p.hlsSegmentCount {
			delete(p.segmentByName, p.segments[0].name)
			p.segments = p.segments[1:]
			p.segmentDeleteCount++
		}
	}()

	p.cond.Broadcast()
}
