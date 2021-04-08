package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"sync"

	"github.com/hajimehoshi/go-mp3"
	"github.com/hajimehoshi/oto"
)

func newSound() (*sound, error) {
	s := &sound{
		messages: make(chan interface{}),
	}
	go s.stream()
	return s, nil
}

type sound struct {
	messages chan interface{}
	statusMu sync.Mutex
	status   soundStatus
}

type soundStatus struct {
	playingFile    bool
	currentFile    string
	fractionPlayed float64
	paused         bool
	errors         []error
}

func (s *soundStatus) appendErr(context string, err error) {
	msg := context + ": "
	if s.currentFile != "" {
		msg += s.currentFile + ": "
	}
	msg += err.Error()
	s.errors = append(s.errors, errors.New(msg))
}

type quit struct{}

type playSong string

type moveToFraction float64

type togglePause struct{}

func (s *sound) stream() {
	var status soundStatus
	updateStatus := func() {
		s.statusMu.Lock()
		s.status = status
		s.statusMu.Unlock()
	}

	const (
		samplesPerSecond = 44100
		sampleChannels   = 2
		bytesPerSample   = 2
		bytesPerSecond   = samplesPerSecond * sampleChannels * bytesPerSample
		// The "/ 4 * 4" is there at the end to make bufferSize divisible by 4.
		// There are 2 channels, samples are 2 bytes in size, thus we need 4
		// byte alignment to not get out of sync and accidentally start playing
		// at an odd bytes offet, that would be noisy.
		bufferSize = bytesPerSecond / 16 / 4 * 4
	)
	context, err := oto.NewContext(
		samplesPerSecond,
		sampleChannels,
		bytesPerSample,
		bufferSize,
	)
	if err != nil {
		status.appendErr("Failed to initialize sound system", err)
		return
	}
	defer context.Close()

	player := context.NewPlayer()
	defer player.Close()

	buf := make([]byte, bufferSize)
	silence := bytes.NewReader(make([]byte, bufferSize))
	var decoder *mp3.Decoder

	for {
		select {
		case msg := <-s.messages:
			switch message := msg.(type) {
			case quit:
				defer func() { s.messages <- quit{} }()
				return
			case playSong:
				decoder = nil              // Stop playing the old song, if any.
				status.playingFile = false // Will be set to true on success.
				status.paused = false
				status.fractionPlayed = 0.0
				status.currentFile = string(message) // Set for error messages.
				data, err := ioutil.ReadFile(status.currentFile)
				if err != nil {
					status.appendErr("Failed to load mp3 file", err)
				} else {
					r, err := mp3.NewDecoder(bytes.NewReader(data))
					if err != nil {
						status.appendErr("Failed to decode mp3 header", err)
					} else if r.SampleRate() != 44100 {
						status.appendErr(
							"Only sample rate 44100 is supported",
							fmt.Errorf("sample rate is %v", r.SampleRate()),
						)
					} else {
						decoder = r
						status.playingFile = true
					}
				}
				if !status.playingFile {
					status.currentFile = ""
				}
				updateStatus()
			case moveToFraction:
				if decoder != nil {
					status.paused = false
					fraction := float64(message)
					pos := int64(fraction * float64(decoder.Length()))
					// We must go to a position that is aligned at the left
					// channel of a 2 channel, 2 bytes per sample signal stream
					// so it must be divisible by 4.
					pos = pos - (pos % 4)
					_, err = decoder.Seek(pos, io.SeekStart)
					if err != nil {
						// TODO Instead do
						//      fail("Failed to move in mp3", err)
						// which not only appends the error but also stops the
						// playback of the current file.
						status.appendErr("Failed to move in mp3", err)
					}
					updateStatus()
				}
			case togglePause:
				status.paused = !status.paused
				updateStatus()
			}
		default:
			if status.paused || decoder == nil {
				silence.Seek(0, io.SeekStart)
				io.Copy(player, silence)
			} else {
				n, err := decoder.Read(buf[:])
				if err != nil {
					if err != io.EOF {
						status.appendErr("Failed to decode mp3 stream", err)
					}
					decoder = nil
					status.playingFile = false
					status.currentFile = ""
				}
				_, err = io.Copy(player, bytes.NewReader(buf[:n]))
				if err != nil {
					status.appendErr("Failed to write data to sound system", err)
					decoder = nil
					status.playingFile = false
					status.currentFile = ""
				}

				if decoder != nil {
					pos, _ := decoder.Seek(0, io.SeekCurrent)
					status.fractionPlayed = float64(pos) / float64(decoder.Length())
				}

				updateStatus()
			}
		}
	}
}

func (s *sound) close() {
	s.messages <- quit{}
	<-s.messages // Receive the quit answer.
}

func (s *sound) play(path string) {
	// Actually starting to play the sound might take a while (open file, decode
	// it, etc.). During this time the user might poll the status. We flag it as
	// playing a file synchronously to prevent the user from thinking the file
	// is not queued yet and trying to play it again.
	s.status.playingFile = true
	s.messages <- playSong(path)
}

func (s *sound) moveToFraction(f float64) {
	s.messages <- moveToFraction(f)
}

func (s *sound) togglePause() {
	s.messages <- togglePause{}
}

// currentStatus returns the last status read from the go routine in which the
// sound is played. Any errors are returned and then cleared from the state,
// meaning you only get each error once in the list of errors in the status.
func (s *sound) currentStatus() soundStatus {
	s.statusMu.Lock()
	status := s.status
	s.status.errors = nil
	s.statusMu.Unlock()
	return status
}
