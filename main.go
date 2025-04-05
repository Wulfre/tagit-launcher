package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	rg "github.com/gen2brain/raylib-go/raygui"
	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	windowWidth  = 600
	windowHeight = 450
	titleSize    = 40
	subtitleSize = 20
	padding      = 10
	buttonWidth  = 300
	buttonHeight = 40
)

type (
	Asset struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	}

	releaseInfo struct {
		Tag    string  `json:"tag_name"`
		Assets []Asset `json:"assets"`
	}

	Button struct {
		text string
		fn   func()
	}

	SemVer struct{ Major, Minor, Patch int }
)

//go:embed fonts/hyperlegible.subset.otf
var fontData []byte

//go:embed winres/icon.png
var iconData []byte

//go:embed styles/dark.rgs
var darkStyleData []byte

var (
	downloadTotal   atomic.Int64
	downloadCurrent atomic.Int64
	cacheDir        = filepath.Join(func() string { dir, _ := os.UserCacheDir(); return dir }(), "tagit")
)

func downloadFiles(tag string, assets []Asset, updateType int) {
	downloadTotal.Store(0)
	downloadCurrent.Store(0)
	os.MkdirAll(cacheDir, 0755)

	var wg sync.WaitGroup
	successfulDownloads := sync.Map{}

	for _, asset := range assets {
		isPckFile := strings.HasSuffix(asset.Name, ".pck")
		isExecutable := strings.Contains(strings.ToLower(asset.Name), runtime.GOOS)
		if (updateType == 1 && isPckFile) || (updateType != 1 && (isPckFile || isExecutable)) {
			downloadTotal.Add(asset.Size)
			wg.Add(1)
			go func(asset Asset) {
				defer wg.Done()
				tempPath := filepath.Join(cacheDir, "_"+asset.Name)

				if resp, err := http.Get(asset.URL); err == nil && resp.StatusCode == http.StatusOK {
					defer resp.Body.Close()
					if out, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY, 0755); err == nil {
						defer out.Close()
						buf := make([]byte, 1024*1024)
						for {
							if n, err := resp.Body.Read(buf); n > 0 {
								if _, err := out.Write(buf[:n]); err != nil {
									return
								}
								downloadCurrent.Add(int64(n))
							} else if err != nil {
								break
							}
						}
						successfulDownloads.Store(asset.Name, tempPath)
					}
				}
			}(asset)
		}
	}

	go func() {
		wg.Wait()
		if downloadCurrent.Load() == downloadTotal.Load() {
			successfulDownloads.Range(func(name, tempPath any) bool {
				finalPath := filepath.Join(cacheDir, name.(string))
				os.Remove(finalPath)
				return os.Rename(tempPath.(string), finalPath) == nil
			})
			os.WriteFile(filepath.Join(cacheDir, "version"), []byte(tag), 0644)
			launchTagIt()
		}
	}()
}

func parseSemVer(version string) (SemVer, error) {
	parts := strings.Split(strings.TrimSuffix(strings.TrimSuffix(version, "x"), "i"), ".")
	if len(parts) != 3 {
		return SemVer{}, fmt.Errorf("invalid version: %s", version)
	}

	v := make([]int, 3)
	for i, p := range parts {
		if v[i], _ = strconv.Atoi(p); v[i] == 0 && p != "0" {
			return SemVer{}, fmt.Errorf("invalid part: %s", p)
		}
	}
	return SemVer{v[0], v[1], v[2]}, nil
}

func main() {
	var release releaseInfo
	if resp, err := http.Get("https://api.github.com/repos/Ketei/tagit-launcher/releases/latest"); err == nil {
		defer resp.Body.Close()
		json.NewDecoder(resp.Body).Decode(&release)
	}

	os.MkdirAll(cacheDir, 0755)
	curVer := strings.TrimSpace(string(func() []byte { b, _ := os.ReadFile(filepath.Join(cacheDir, "version")); return b }()))

	isFirstLaunch := curVer == ""
	subtitle := "Unable to check for updates."
	updateType := -1

	if tag := release.Tag; tag != "" {
		if isFirstLaunch {
			subtitle = "Version " + tag + " available"
		} else if curVer == tag || strings.HasSuffix(curVer, "x") ||
			(strings.HasSuffix(curVer, "i") && strings.TrimSuffix(curVer, "i") == tag) {
			go launchTagIt()
		} else {
			old, err1 := parseSemVer(curVer)
			new, err2 := parseSemVer(tag)
			if err1 == nil && err2 == nil {
				switch {
				case old.Major != new.Major:
					updateType, subtitle = 3, "Full update "+tag+" available"
				case old.Minor != new.Minor:
					updateType, subtitle = 2, "Full update "+tag+" available"
				case old.Patch != new.Patch:
					updateType, subtitle = 1, "Patch update "+tag+" available"
				default:
					go launchTagIt()
				}
			} else {
				subtitle = "Update " + tag + " available"
			}
		}
	}

	rl.InitWindow(windowWidth, windowHeight, "TagIt Launcher")
	rl.SetTargetFPS(60)
	defer rl.CloseWindow()

	rg.LoadStyleFromMemory(darkStyleData)
	font := rl.LoadFontFromMemory(".otf", fontData, int32(titleSize), nil)
	rl.SetTextureFilter(font.Texture, rl.FilterBilinear)
	defer rl.UnloadFont(font)

	icon := rl.LoadImageFromMemory(".png", iconData, int32(len(iconData)))
	rl.SetWindowIcon(*icon)
	rl.UnloadImage(icon)

	rg.SetFont(font)
	rg.SetStyle(rg.DEFAULT, rg.TEXT_SIZE, 16)

	headerHeight := float32(padding + titleSize + padding + subtitleSize)
	btnSpacing := (float32(windowHeight) - headerHeight - float32(buttonHeight*5)) / 6
	titlePos := rl.NewVector2(
		float32(windowWidth)/2-rl.MeasureTextEx(font, "TagIt", titleSize, 1).X/2,
		float32(padding))

	textColor := rl.GetColor(uint(rg.GetStyle(rg.DEFAULT, rg.TEXT_COLOR_NORMAL)))
	bgColor := rl.GetColor(uint(rg.GetStyle(rg.DEFAULT, rg.BACKGROUND_COLOR)))

	buttons := []Button{
		{text: map[bool]string{true: "Install TagIt", false: "Download Update"}[isFirstLaunch],
			fn: func() {
				if release.Tag != "" {
					downloadFiles(release.Tag, release.Assets, updateType)
				}
			}},
		{"Skip Update", func() {
			os.WriteFile(filepath.Join(cacheDir, "version"), []byte(release.Tag+"i"), 0644)
			launchTagIt()
		}},
		{"Disable Updates", func() {
			os.WriteFile(filepath.Join(cacheDir, "version"), []byte(release.Tag+"x"), 0644)
			launchTagIt()
		}},
		{"Remind Me Later", launchTagIt},
		{"Exit", func() { os.Exit(0) }},
	}

	for !rl.WindowShouldClose() {
		rl.BeginDrawing()
		rl.ClearBackground(bgColor)

		rl.DrawTextEx(font, "TagIt", titlePos, titleSize, 1, textColor)

		if downloading := downloadTotal.Load() > 0; downloading {
			percent := float32(downloadCurrent.Load()) / float32(downloadTotal.Load())
			text := fmt.Sprintf("%.0f%%", percent*100)

			rg.Enable()
			rg.ProgressBar(
				rl.NewRectangle(
					float32(padding*2),
					headerHeight-float32(subtitleSize)-float32(padding),
					float32(windowWidth)-float32(padding*4),
					20,
				),
				"", "", percent, 0, 1,
			)

			rl.DrawTextEx(font, text,
				rl.NewVector2(
					float32(windowWidth)/2-rl.MeasureTextEx(font, text, subtitleSize*0.8, 1).X/2,
					headerHeight-float32(subtitleSize)*1.4,
				),
				subtitleSize*0.8, 1, textColor,
			)
			rg.Disable()
		} else {
			rl.DrawTextEx(font, subtitle,
				rl.NewVector2(
					float32(windowWidth)/2-rl.MeasureTextEx(font, subtitle, subtitleSize, 1).X/2,
					float32(padding+titleSize+padding),
				),
				subtitleSize, 1, textColor,
			)
		}

		for i, btn := range buttons {
			if isFirstLaunch && i > 0 && i < len(buttons)-1 {
				rg.Disable()
			}

			if rg.Button(
				rl.NewRectangle(
					float32(windowWidth)/2-float32(buttonWidth)/2,
					headerHeight+btnSpacing*(float32(i)+1)+float32(buttonHeight*i),
					float32(buttonWidth),
					float32(buttonHeight),
				),
				btn.text,
			) && downloadTotal.Load() == 0 {
				btn.fn()
			}

			if isFirstLaunch && i > 0 && i < len(buttons)-1 {
				rg.Enable()
			}
		}

		rl.EndDrawing()
	}
}
