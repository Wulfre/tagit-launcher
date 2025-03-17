package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	rg "github.com/gen2brain/raylib-go/raygui"
	rl "github.com/gen2brain/raylib-go/raylib"
)

//go:embed embed/fonts/hyperlegible.subset.otf
var fontData []byte

// ui constants
const (
	windowWidth  = 600
	windowHeight = 450
	titleSize    = 40
	subtitleSize = 20
	padding      = 10
	buttonWidth  = 300
	buttonHeight = 40
)

type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type releaseInfo struct {
	Tag    string  `json:"tag_name"`
	Assets []Asset `json:"assets"`
}

type Button struct {
	text string
	fn   func()
}

type SemVer struct {
	Major int
	Minor int
	Patch int
}

// ParseSemVer parses a version string into a SemVer struct, assumes version is always in the format x.y.z
func ParseSemVer(version string) (SemVer, error) {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return SemVer{}, fmt.Errorf("invalid semantic version: %s", version)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return SemVer{}, fmt.Errorf("invalid major version: %s", parts[0])
	}

	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return SemVer{}, fmt.Errorf("invalid minor version: %s", parts[1])
	}

	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return SemVer{}, fmt.Errorf("invalid patch version: %s", parts[2])
	}

	return SemVer{Major: major, Minor: minor, Patch: patch}, nil
}

// CompareVersions compares two versions and returns:
// 0 - versions are equal
// 1 - oldVersion has only patch difference (lower patch)
// 2 - oldVersion has minor or major difference (lower minor/major)
// -1 - error parsing versions
func CompareVersions(oldVersion, newVersion string) int {
	// normalize special suffixes
	oldVer := strings.TrimSuffix(strings.TrimSuffix(oldVersion, "x"), "i")

	old, err := ParseSemVer(oldVer)
	if err != nil {
		return -1
	}

	new, err := ParseSemVer(newVersion)
	if err != nil {
		return -1
	}

	if old.Major != new.Major {
		return 3
	}

	if old.Minor != new.Minor {
		return 2
	}

	if old.Patch != new.Patch {
		return 1
	}

	// versions are the same
	return 0
}

var (
	downloadTotal   atomic.Int64
	downloadCurrent atomic.Int64
	cacheDir        = filepath.Join(func() string { dir, _ := os.UserCacheDir(); return dir }(), "tagit")
)

func launchTagIt() {
	execName := "tagit." + runtime.GOOS + ".x86_64"
	if runtime.GOOS == "windows" {
		execName += ".exe"
	}
	execPath := filepath.Join(cacheDir, execName)
	if _, err := os.Stat(execPath); err == nil {
		exec.Command(execPath).Start()
		os.Exit(0)
	}
}

// downloads the application assets for the current platform, downloads are performed concurrently and progress is tracked atomically
func downloadFiles(tag string, assets []Asset, updateType int) {
	downloadTotal.Store(0)
	downloadCurrent.Store(0)
	os.MkdirAll(cacheDir, 0755)

	var wg sync.WaitGroup
	downloadMap := make(map[string]bool)
	var downloadMapMutex sync.Mutex

	// calculate total download size and create file mapping
	for _, a := range assets {
		isPckFile := strings.HasSuffix(a.Name, ".pck")
		isExecutable := strings.Contains(strings.ToLower(a.Name), runtime.GOOS)

		if (updateType == 1 && isPckFile) ||
			(updateType != 1 && (isPckFile || isExecutable)) {
			downloadTotal.Add(a.Size)
			wg.Add(1)

			go func(name, url string) {
				defer wg.Done()

				tempFilePath := filepath.Join(cacheDir, "_"+name)

				resp, err := http.Get(url)
				if err != nil || resp.StatusCode != http.StatusOK {
					return
				}
				defer resp.Body.Close()

				out, err := os.OpenFile(tempFilePath, os.O_CREATE|os.O_WRONLY, 0755)
				if err != nil {
					return
				}
				defer out.Close()

				buf := make([]byte, 1024*1024)
				success := true
				for {
					n, err := resp.Body.Read(buf)
					if n > 0 {
						_, writeErr := out.Write(buf[:n])
						if writeErr != nil {
							success = false
							break
						}
						downloadCurrent.Add(int64(n))
					}
					if err != nil {
						break
					}
				}

				// mark this file as successfully downloaded
				if success {
					downloadMapMutex.Lock()
					downloadMap[name] = true
					downloadMapMutex.Unlock()
				}
			}(a.Name, a.URL)
		}
	}

	// wait for all downloads to complete and launch the application
	go func() {
		wg.Wait()

		if downloadCurrent.Load() == downloadTotal.Load() {
			allSuccessful := true

			for name := range downloadMap {
				tempPath := filepath.Join(cacheDir, "_"+name)
				finalPath := filepath.Join(cacheDir, name)

				os.Remove(finalPath)

				if err := os.Rename(tempPath, finalPath); err != nil {
					allSuccessful = false
					break
				}
			}

			if allSuccessful {
				os.WriteFile(filepath.Join(cacheDir, "version"), []byte(tag), 0644)
				launchTagIt()
			}
		}
	}()
}

func main() {
	// fetch latest release information from GitHub
	var release releaseInfo
	if resp, err := http.Get("https://api.github.com/repos/Ketei/tagit-launcher/releases/latest"); err == nil {
		defer resp.Body.Close()
		json.NewDecoder(resp.Body).Decode(&release)
	}

	// check current version and determine update status
	os.MkdirAll(cacheDir, 0755)
	curVer := strings.TrimSpace(string(func() []byte { b, _ := os.ReadFile(filepath.Join(cacheDir, "version")); return b }()))
	subtitle := "Unable to check for updates."

	// default to full download for first install
	updateType := -1

	// compare local version with latest release
	if release.Tag != "" {
		switch {
		case curVer == "": // first run, show update available
			subtitle = "Version " + release.Tag + " available"
		case curVer == release.Tag || strings.HasSuffix(curVer, "x") || // already up to date or updates disabled
			(strings.HasSuffix(curVer, "i") && strings.TrimSuffix(curVer, "i") == release.Tag): // update ignored
			go launchTagIt()
		default: // update available
			updateType = CompareVersions(curVer, release.Tag)
			switch updateType {
			case 0:
				// versions are the same, no update needed
				go launchTagIt()
			case 1:
				subtitle = "Patch update " + release.Tag + " available"
			case 2, 3:
				subtitle = "Full update " + release.Tag + " available"
			default:
				subtitle = "Update " + release.Tag + " available"
			}
		}
	}

	// initialize window
	rl.InitWindow(windowWidth, windowHeight, "TagIt Launcher")
	defer rl.CloseWindow()
	rl.SetTargetFPS(60)

	// set up custom font and ui styling
	font := rl.LoadFontFromMemory(".otf", fontData, int32(titleSize), nil)
	defer rl.UnloadFont(font)
	rl.SetTextureFilter(font.Texture, rl.FilterBilinear)
	rg.SetFont(font)
	rg.SetStyle(rg.DEFAULT, rg.TEXT_SIZE, 16)

	// get ui colors from the current theme
	textColor := rl.GetColor(uint(rg.GetStyle(rg.DEFAULT, rg.TEXT_COLOR_NORMAL)))
	bgColor := rl.GetColor(uint(rg.GetStyle(rg.DEFAULT, rg.BACKGROUND_COLOR)))

	// define ui buttons and their actions
	isFirstLaunch := curVer == ""
	buttonText := "Download Update"
	if isFirstLaunch {
		buttonText = "Install TagIt"
	}

	buttons := []Button{
		{buttonText, func() {
			if release.Tag != "" {
				downloadFiles(release.Tag, release.Assets, updateType)
			}
		}},
		{"Skip Update", func() { os.WriteFile(filepath.Join(cacheDir, "version"), []byte(release.Tag+"i"), 0644); launchTagIt() }},
		{"Disable Updates", func() { os.WriteFile(filepath.Join(cacheDir, "version"), []byte(release.Tag+"x"), 0644); launchTagIt() }},
		{"Remind Me Later", launchTagIt},
		{"Exit", func() { os.Exit(0) }},
	}

	// calculate layout measurements
	headerHeight := float32(padding + titleSize + padding + subtitleSize)
	btnSpacing := (float32(windowHeight) - headerHeight - float32(buttonHeight*5)) / 6
	titlePos := rl.NewVector2(float32(windowWidth)/2-rl.MeasureTextEx(font, "TagIt", titleSize, 1).X/2, float32(padding))

	// main render loop
	for !rl.WindowShouldClose() {
		rl.BeginDrawing()
		rl.ClearBackground(bgColor)

		// draw title
		rl.DrawTextEx(font, "TagIt", titlePos, titleSize, 1, textColor)

		// make sure gui is enabled for progress bar
		rg.Enable()

		// draw progress bar or subtitle
		if downloading := downloadTotal.Load() > 0; downloading {
			percent := float32(downloadCurrent.Load()) / float32(downloadTotal.Load())
			text := fmt.Sprintf("%.0f%%", percent*100)

			// draw progress bar and percentage
			rg.ProgressBar(rl.NewRectangle(float32(padding*2), headerHeight-float32(subtitleSize)-float32(padding),
				float32(windowWidth)-float32(padding*4), 20), "", "", percent, 0, 1)
			rl.DrawTextEx(font, text,
				rl.NewVector2(float32(windowWidth)/2-rl.MeasureTextEx(font, text, subtitleSize*0.8, 1).X/2,
					headerHeight-float32(subtitleSize)*1.4),
				subtitleSize*0.8, 1, textColor)

			// only disable after drawing progress bar
			rg.Disable()
		} else {
			// draw update status
			rl.DrawTextEx(font, subtitle,
				rl.NewVector2(float32(windowWidth)/2-rl.MeasureTextEx(font, subtitle, subtitleSize, 1).X/2,
					float32(padding+titleSize+padding)),
				subtitleSize, 1, textColor)
		}

		// draw buttons
		for i, btn := range buttons {
			if isFirstLaunch && i > 0 && i < len(buttons)-1 {
				// disable all buttons except the first and last (install and exit) on first launch
				rg.Disable()
			}

			if rg.Button(rl.NewRectangle(float32(windowWidth)/2-float32(buttonWidth)/2,
				headerHeight+btnSpacing*(float32(i)+1)+float32(buttonHeight*i),
				float32(buttonWidth), float32(buttonHeight)), btn.text) && downloadTotal.Load() == 0 {
				btn.fn()
			}

			if isFirstLaunch && i > 0 && i < len(buttons)-1 {
				rg.Enable()
			}
		}

		rl.EndDrawing()
	}
}
