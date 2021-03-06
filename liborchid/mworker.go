package liborchid

import (
	"sync"
	"time"
)

type PlaybackResult struct {
	Song     *Song
	Stream   *Stream
	Complete bool
	Error    error
}

type MWorker struct {
	mux          sync.Mutex
	stream       *Stream
	volume       VolumeInfo
	VolumeChange chan VolumeInfo
	Results      chan *PlaybackResult
	SongQueue    chan *Song
	Progress     chan float64
	stop         chan struct{}
}

func NewMWorker() *MWorker {
	return &MWorker{
		VolumeChange: make(chan VolumeInfo),
		Results:      make(chan *PlaybackResult),
		SongQueue:    make(chan *Song),
		Progress:     make(chan float64),
		stop:         make(chan struct{}),
		volume: VolumeInfo{
			V:   0,
			Min: -1,
			Max: 0,
		},
	}
}

func (mw *MWorker) VolumeInfo() VolumeInfo {
	mw.mux.Lock()
	defer mw.mux.Unlock()
	return mw.volume
}

func (mw *MWorker) setVolume(v VolumeInfo) {
	mw.mux.Lock()
	defer mw.mux.Unlock()
	mw.volume = v
	if mw.stream != nil {
		mw.stream.SetVolume(v)
	}
}

func (mw *MWorker) setStream(stream *Stream) {
	mw.mux.Lock()
	defer mw.mux.Unlock()
	mw.stream = stream
	if stream != nil {
		stream.SetVolume(mw.volume)
	}
}

func (mw *MWorker) Stream() *Stream {
	mw.mux.Lock()
	defer mw.mux.Unlock()
	return mw.stream
}

func (mw *MWorker) Stop() {
	mw.stop <- struct{}{}
}

func (mw *MWorker) report(stream *Stream, song *Song, complete bool, err error) {
	go func() {
		mw.Results <- &PlaybackResult{
			Song:     song,
			Stream:   stream,
			Complete: complete,
			Error:    err,
		}
	}()
}

func (mw *MWorker) playSong(song *Song) {
	// If there's a current stream we need to stop it first so that
	// there is no leaked channels.
	if s := mw.Stream(); s != nil {
		s.Stop()
	}
	if song == nil {
		return
	}
	// continue playing the next stream
	stream, err := song.Stream()
	if err != nil {
		mw.report(nil, song, false, err)
		return
	}
	mw.setStream(stream)
	stream.Play()
	go func() {
		mw.Progress <- 0.0
		t := time.NewTicker(time.Second * 1)
		c := stream.Complete()
		for {
			select {
			case d := <-c:
				mw.report(stream, song, d, nil)
				mw.setStream(nil)
				t.Stop()
				return
			case <-t.C:
				mw.Progress <- stream.Progress()
			}
		}
	}()
}

func (mw *MWorker) Play() {
	d := time.Millisecond * 50
	t := time.NewTimer(d)
	t.Stop()
	var song *Song
	for {
		select {
		case song = <-mw.SongQueue:
			t.Reset(d)
		case <-t.C:
			mw.playSong(song)
		case vol := <-mw.VolumeChange:
			mw.setVolume(vol)
		case <-mw.stop:
			return
		}
	}
}
