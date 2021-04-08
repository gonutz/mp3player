package main

import (
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gonutz/prototype/draw"
)

func main() {
	sound, err := newSound()
	if err != nil {
		panic(err)
	}
	defer sound.close()

	var (
		workingDir              = "."
		textScale               = float32(1.0)
		fullscreen              = false
		foregroundColor         = rgb(206, 200, 176)
		backgroundColor         = rgb(7, 39, 39)
		highlightColor          = rgb(0, 0, 255)
		files                   = []string{}
		filesY                  = 0.0
		highlightedFile         = -1
		highlightedPlaylistItem = -1
		playlist                = []string{}
		playlistY               = 0.0
		filesFilter             = ""
		playlistFilter          = ""
		playing                 = ""
		playingIndex            = -1
	)
	if user, err := user.Current(); err == nil {
		workingDir = user.HomeDir
	}
	musicDir := filepath.Join(workingDir, "Music")
	if _, err := os.Stat(musicDir); err == nil {
		workingDir = musicDir
	}
	files = listFoldersAndMp3files(workingDir)

	frames := 0
	err = draw.RunWindow("Music Player", 1500, 800, func(window draw.Window) {
		frames++

		play := func(index int) {
			n := len(playlist)
			if n != 0 {
				index += n
				index %= n
			}
			if 0 <= index && index < n {
				playingIndex = index
				playing = playlist[index]
				// TODO The working dir might have changed already, make the
				// playlist not simply an array of strings but paths.
				sound.play(filepath.Join(workingDir, playing))
			} else {
				playingIndex = -1
				playing = ""
			}
		}
		status := sound.currentStatus()
		if !status.playingFile {
			// TODO Jump randomly once that is an option.
			play(playingIndex + 1)
		}

		drawText := func(text string, x, y int) {
			window.DrawScaledText(text, x, y, textScale, foregroundColor)
		}
		textWidth := func(text string) int {
			w, _ := window.GetScaledTextSize(text, textScale)
			return w
		}
		textHeight := func(text string) int {
			_, h := window.GetScaledTextSize(text, textScale)
			return h
		}
		charW := textWidth("_")
		lineH := textHeight("A")
		frame := func(r rectangle) {
			window.DrawRect(r.x, r.y, r.w, r.h, foregroundColor)
		}
		fill := func(r rectangle) {
			window.FillRect(r.x, r.y, r.w, r.h, backgroundColor)
		}
		highlight := func(r rectangle) {
			window.FillRect(r.x, r.y, r.w, r.h, highlightColor)
		}

		// Compute where we will draw UI elements.
		windowW, windowH := window.Size()
		pathRect := rect(0, 0, windowW/2, 2*lineH)
		filesFilterRect := pathRect
		filesFilterRect.y = windowH - filesFilterRect.h
		filesRect := rect(
			pathRect.x,
			pathRect.bottom(),
			pathRect.w,
			filesFilterRect.y-pathRect.bottom()+1,
		)
		playlistRect := filesRect
		playlistRect.x = filesRect.right()
		playlistRect.w = windowW - windowW/2
		playlistFilterRect := filesFilterRect
		playlistFilterRect.x = playlistRect.x
		playlistFilterRect.w = playlistRect.w
		controlsRect := playlistFilterRect
		controlsRect.y = 0

		leftRect := rect(0, 0, playlistRect.w, windowH)

		// Draw the UI.
		fill(filesRect)
		if highlightedFile != -1 {
			y := filesRect.y + highlightedFile*lineH + int(filesY)
			window.FillRect(filesRect.x, y, filesRect.w, lineH, highlightColor)
		}
		drawText(
			strings.Join(files, "\n"),
			filesRect.x+charW,
			filesRect.y+int(filesY),
		)
		frame(filesRect)

		fill(playlistRect)
		if highlightedPlaylistItem != -1 {
			y := playlistRect.y + highlightedPlaylistItem*lineH + int(playlistY)
			window.FillRect(playlistRect.x, y, playlistRect.w, lineH, highlightColor)
		}
		drawText(
			strings.Join(playlist, "\n"),
			playlistRect.x+charW,
			playlistRect.y+int(playlistY),
		)
		frame(playlistRect)

		fill(pathRect)
		path := " " + workingDir + " "
		maxRunes := pathRect.w / charW
		if maxRunes > len(" ... ") {
			runes := []rune(path)
			if len(runes) > maxRunes {
				left := maxRunes / 2
				right := maxRunes - left
				left -= 2  // Leave three spaces...
				right -= 1 // ... for the ellipsis.
				path = string(runes[:left]) + "..." + string(runes[len(runes)-right:])
			}
			drawText(path, pathRect.x+(pathRect.w-textWidth(path))/2, pathRect.y+lineH/2)
		}
		frame(pathRect)

		fill(filesFilterRect)
		drawText(
			" Filter: "+filesFilter,
			filesFilterRect.x,
			filesFilterRect.y+lineH/2,
		)
		frame(filesFilterRect)

		fill(playlistFilterRect)
		drawText(
			" Filter: "+playlistFilter,
			playlistFilterRect.x,
			playlistFilterRect.y+lineH/2,
		)
		frame(playlistFilterRect)

		fill(controlsRect)
		if !(status.paused && (frames/30)%2 == 0) {
			played := controlsRect
			played.w = int(float64(played.w) * status.fractionPlayed)
			highlight(played)
		}
		drawText(" "+playing, controlsRect.x, controlsRect.y+lineH/2)
		frame(controlsRect)

		// Handle input.
		if window.WasKeyPressed(draw.KeyEscape) {
			window.Close()
		}
		if window.WasKeyPressed(draw.KeyF11) {
			fullscreen = !fullscreen
			window.SetFullscreen(fullscreen)
		}

		if window.WasKeyPressed(draw.KeyF12) {
			textScale += 0.5
		}
		if window.WasKeyPressed(draw.KeyF10) {
			textScale -= 0.5
			if textScale < 1 {
				textScale = 1
			}
		}

		if window.WasKeyPressed(draw.KeyF6) {
			play(playingIndex - 1)
		}
		if window.WasKeyPressed(draw.KeyF7) {
			sound.togglePause()
		}
		if window.WasKeyPressed(draw.KeyF8) {
			play(playingIndex + 1)
		}

		mouseX, mouseY := window.MousePosition()
		scrollY := &playlistY
		if leftRect.contains(mouseX, mouseY) {
			scrollY = &filesY
		}
		if window.WasKeyPressed(draw.KeyPageDown) {
			*scrollY -= float64(windowH) * 0.8
		}
		if window.WasKeyPressed(draw.KeyPageUp) {
			*scrollY += float64(windowH) * 0.8
		}
		if window.WasKeyPressed(draw.KeyHome) {
			*scrollY = 0
		}
		if window.WasKeyPressed(draw.KeyEnd) {
			*scrollY = -99999999
		}
		*scrollY += window.MouseWheelY() * float64(lineH)

		filesY = clamp(
			-float64(len(files)*lineH-filesRect.h),
			filesY,
			0,
		)
		playlistY = clamp(
			-float64(len(playlist)*lineH-playlistRect.h),
			playlistY,
			0,
		)

		if window.WasKeyPressed(draw.KeyF5) {
			// TODO Re-filter the files.
			files = listFoldersAndMp3files(workingDir)
			filesFilter = ""
		}

		oldFilesFilter := filesFilter
		var unused string
		updateText := &unused
		if leftRect.contains(mouseX, mouseY) {
			updateText = &filesFilter
		} else {
			updateText = &playlistFilter
		}
		for _, r := range window.Characters() {
			if r == '\b' {
				_, size := utf8.DecodeLastRuneInString(*updateText)
				*updateText = (*updateText)[:len(*updateText)-size]
			} else if unicode.IsPrint(r) {
				*updateText += string(r)
			}
		}
		if filesFilter != oldFilesFilter {
			files = listFoldersAndMp3files(workingDir)
			f := newFilter(filesFilter)
			n := 0
			for i := range files {
				if files[i] == ".." || f.fits(files[i]) {
					files[n] = files[i]
					n++
				}
			}
			files = files[:n]
		}

		if clicks := window.Clicks(); len(clicks) > 0 {
			c := clicks[0]
			controlDown := window.IsKeyDown(draw.KeyLeftControl) ||
				window.IsKeyDown(draw.KeyRightControl)

			if controlDown && c.Button == draw.LeftButton &&
				filesRect.contains(c.X, c.Y) {
				// Add all songs on Ctrl+Left Click.
				for _, f := range files {
					if isMp3(f) {
						playlist = append(playlist, f)
						if len(playlist) == 1 && !status.playingFile {
							play(0)
						}
					}
				}
			} else if c.Button == draw.LeftButton && highlightedFile != -1 {
				f := files[highlightedFile]
				if isMp3(f) {
					playlist = append(playlist, f)
					if len(playlist) == 1 && !status.playingFile {
						play(0)
					}
				} else {
					workingDir = filepath.Join(workingDir, f)
					files = listFoldersAndMp3files(workingDir)
				}
			} else if c.Button == draw.LeftButton && controlsRect.contains(c.X, c.Y) {
				fraction := float64(c.X-controlsRect.x) / float64(controlsRect.w)
				sound.moveToFraction(fraction)
			} else if controlDown && c.Button == draw.RightButton &&
				playlistRect.contains(c.X, c.Y) {
				playlist = nil
			} else if highlightedPlaylistItem != -1 {
				i := highlightedPlaylistItem
				if c.Button == draw.LeftButton {
					play(i)
				} else if c.Button == draw.RightButton {
					playlist = append(playlist[:i], playlist[i+1:]...)
				}
			}
		}

		highlightedFile = -1
		if filesRect.contains(mouseX, mouseY) {
			i := (mouseY - filesRect.y - int(filesY)) / lineH
			if 0 <= i && i < len(files) {
				highlightedFile = i
			}
		}

		highlightedPlaylistItem = -1
		if playlistRect.contains(mouseX, mouseY) {
			i := (mouseY - playlistRect.y - int(playlistY)) / lineH
			if 0 <= i && i < len(playlist) {
				highlightedPlaylistItem = i
			}
		}
	})
	if err != nil {
		panic(err)
	}
}

func listFoldersAndMp3files(path string) []string {
	files, err := ioutil.ReadDir(path)
	var dirs, mp3files []string
	dirs = []string{".."}
	if err == nil {
		for _, f := range files {
			if f.IsDir() {
				dirs = append(dirs, f.Name())
			} else if isMp3(f.Name()) {
				mp3files = append(mp3files, f.Name())
			}
		}
	}
	sort.Slice(mp3files, func(i, j int) bool {
		return strings.ToLower(mp3files[i]) < strings.ToLower(mp3files[j])
	})
	return append(dirs, mp3files...)
}

func isMp3(f string) bool {
	return strings.HasSuffix(strings.ToLower(f), ".mp3")
}

func newFilter(pattern string) *filter {
	parts := strings.Split(pattern, " ")
	for i := range parts {
		parts[i] = strings.ToLower(parts[i])
	}
	return &filter{parts: parts}
}

type filter struct {
	parts []string
}

func (f *filter) fits(s string) bool {
	s = strings.ToLower(s)
	for _, p := range f.parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}

func clamp(min, x, max float64) float64 {
	if x < min {
		x = min
	}
	if x > max {
		x = max
	}
	return x
}

type rectangle struct {
	x, y, w, h int
}

func rect(x, y, w, h int) rectangle {
	return rectangle{x: x, y: y, w: w, h: h}
}

func (r *rectangle) right() int {
	return r.x + r.w - 1
}

func (r *rectangle) bottom() int {
	return r.y + r.h - 1
}

func (r *rectangle) contains(x, y int) bool {
	return r.x <= x && x <= r.right() && r.y <= y && y <= r.bottom()
}

func rgb(r, g, b byte) draw.Color {
	x := func(n byte) float32 { return float32(n) / 255.0 }
	return draw.RGB(x(r), x(g), x(b))
}
