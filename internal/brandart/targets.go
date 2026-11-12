// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package brandart

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
)

// Files is what a target writes: a path inside the repository (always with forward slashes) and
// the file's bytes.
type Files map[string][]byte

// Paths is the files' paths, sorted.
func (f Files) Paths() []string {
	var out []string
	for p := range f {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// Targets is every target genicon knows: one per repository, and the server's packaging set.
var Targets = []string{"server", "server-package", "client", "player", "reader", "catalog", "website", "kit"}

// Build makes a target's files. modules is the catalogue's id → category, for "catalog".
func (k Kit) Build(target string, modules map[string]string) (Files, error) {
	f := Files{}
	switch target {
	case "server":
		k.webIcons(f, Server, "web/public/")
	case "server-package":
		k.serverPackage(f)
	case "client":
		k.apple(f)
		k.android(f)
		k.desktop(f, Client, "desktop/build/")
	case "player":
		f["resources/WittyMediaPlayer.ico"] = ICO(WindowsSizes, func(px int) Art { return k.Tile(Player, px) })
		f["resources/icon.svg"] = k.Tile(Player, 0).SVG()
		f["resources/icon-256.png"] = k.Tile(Player, 256).PNG(256, 256)
		f["resources/windows/banner.bmp"] = BMP(k.InstallerBanner(Player, 493, 58).Image(493, 58))
		f["resources/windows/welcome.bmp"] = BMP(k.wizardWelcome(Player).Image(493, 312))
	case "reader":
		k.webIcons(f, Reader, "public/")
		k.desktop(f, Reader, "desktop/build/")
	case "catalog":
		if len(modules) == 0 {
			return nil, fmt.Errorf("catalog: no modules (genicon reads them from catalog.json)")
		}
		for id, cat := range modules {
			img := k.ModuleIcon(id, cat, 128)
			if img == nil {
				return nil, fmt.Errorf("catalog: %s (%s) has no mark or no category colour: add it to internal/brandart/modules.go", id, cat)
			}
			f["icons/"+id+".png"] = encodePNG(img)
		}
	case "kit":
		return k.BuildKit(modules), nil
	case "website":
		k.webIcons(f, Server, "images/brand/")
		f["images/brand/social.png"] = k.Social(Server, 1200, 630, "Your films, shows, music, books and games, served from home.").PNG(1200, 630)
		f["images/brand/hero-dark.svg"] = k.Hero(1600, 900, false).SVG()
		f["images/brand/hero-light.svg"] = k.Hero(1600, 900, true).SVG()
		for _, p := range Products {
			f["images/brand/"+string(p)+".svg"] = k.Tile(p, 0).SVG()
		}
	default:
		return nil, fmt.Errorf("no target %q (have %v)", target, Targets)
	}
	return f, nil
}

// webIcons are a web app's favicon, manifest icons (plain and maskable) and home-screen icon.
func (k Kit) webIcons(f Files, p Product, dir string) {
	f[dir+"icon.svg"] = k.Tile(p, 0).SVG()
	f[dir+"favicon.ico"] = ICO([]int{16, 32, 48}, func(px int) Art { return k.Tile(p, px) })
	f[dir+"icon-192.png"] = k.Tile(p, 192).PNG(192, 192)
	f[dir+"icon-512.png"] = k.Tile(p, 512).PNG(512, 512)
	// Maskable: the platform cuts its own shape, so the background fills the square and the
	// glyph stays inside the middle 80%.
	f[dir+"icon-maskable-512.png"] = k.Full(p, 512).PNG(512, 512)
	f[dir+"apple-touch-icon.png"] = encodePNG(Opaque(k.Full(p, 180).Image(180, 180), k.Pal.Night))
}

// serverPackage is everything the server's installers and packages take, written into one
// folder by scripts/build-release.sh.
func (k Kit) serverPackage(f Files) {
	f["icon.ico"] = ICO(WindowsSizes, func(px int) Art { return k.Tile(Server, px) })
	f["icon.icns"] = ICNS(func(px int) Art { return k.MacTile(Server, px) })
	f["icon.png"] = k.MacTile(Server, 512).PNG(512, 512)
	f["wittymediaserver.png"] = k.Tile(Server, 256).PNG(256, 256)
	f["wittymediaserver.svg"] = k.Tile(Server, 0).SVG()
	for _, s := range []int{16, 22, 24, 32, 48, 64, 128, 256, 512} {
		f[fmt.Sprintf("hicolor/%dx%d/apps/wittymediaserver.png", s, s)] = k.Tile(Server, s).PNG(s, s)
	}
	f["hicolor/scalable/apps/wittymediaserver.svg"] = k.Tile(Server, 0).SVG()
	// Windows Installer's wizard: the strip along the top of its pages, and the first and last
	// pages' picture.
	f["banner.bmp"] = BMP(k.InstallerBanner(Server, 493, 58).Image(493, 58))
	f["welcome.bmp"] = BMP(k.wizardWelcome(Server).Image(493, 312))
	// The macOS installer's background, drawn at 2× and laid out for its 620×418 window.
	f["pkg-background.png"] = k.Scene(1240, 836, 19, true, Box{40, 520, 300, 780}).With(k.Tile(Server, 300).Placed(0, 0, Pt{170, 650}, 260/Page).Layers...).PNG(1240, 836)
}

// wizardWelcome is Windows Installer's 493×312 first page: a picture panel 164 wide on the left,
// white where the words go.
func (k Kit) wizardWelcome(p Product) Art {
	a := Art{W: 493, H: 312, Layers: []Layer{{Rect(0, 0, 493, 312, 0), Fill(Hex("#ffffff"))}}}
	return a.With(k.InstallerPanel(p, 164, 312).Layers...)
}

// desktop is an Electron app's build resources, where electron-builder looks for them: the
// icons for each system, the disk image's background and the Windows installer's pictures.
func (k Kit) desktop(f Files, p Product, dir string) {
	f[dir+"icon.png"] = k.Tile(p, 1024).PNG(1024, 1024)
	f[dir+"icon.icns"] = ICNS(func(px int) Art { return k.MacTile(p, px) })
	f[dir+"icon.ico"] = ICO(WindowsSizes, func(px int) Art { return k.Tile(p, px) })
	for _, s := range []int{16, 24, 32, 48, 64, 128, 256, 512} {
		f[fmt.Sprintf("%sicons/%dx%d.png", dir, s, s)] = k.Tile(p, s).PNG(s, s)
	}
	// The disk image's window is 540×400 with the app at (140, 150) and Applications at (400, 150).
	f[dir+"background.png"] = k.DiskImage(540, 400, 1, 140, 400, 150).PNG(540, 400)
	f[dir+"background@2x.png"] = k.DiskImage(540, 400, 2, 140, 400, 150).PNG(1080, 800)
	side := BMP(k.InstallerPanel(p, 164, 314).Image(164, 314))
	f[dir+"installerSidebar.bmp"] = side
	f[dir+"uninstallerSidebar.bmp"] = side
	f[dir+"installerHeader.bmp"] = BMP(k.InstallerBanner(p, 150, 57).Image(150, 57))
}

// ---- Apple ----

var xcodeInfo = map[string]any{"author": "xcode", "version": 1}

func contents(v map[string]any) []byte {
	v["info"] = xcodeInfo
	b, _ := json.MarshalIndent(v, "", "  ")
	return append(b, '\n')
}

func (k Kit) apple(f Files) {
	ios := "apple/Brand/iOS.xcassets/"
	tv := "apple/Brand/tvOS.xcassets/"
	f[ios+"Contents.json"] = contents(map[string]any{})
	f[tv+"Contents.json"] = contents(map[string]any{})

	// iPhone and iPad: one 1024 picture, and the dark and tinted ones iOS 18 on offers.
	set := ios + "AppIcon.appiconset/"
	f[set+"AppIcon.png"] = encodePNG(Opaque(k.Full(Client, 1024).Image(1024, 1024), k.Pal.Night))
	darkBack := Art{W: Page, H: Page, Layers: []Layer{{square(), Linear(Pt{0, 0}, Pt{0, Page}, Mix(k.Pal.Night, k.Pal.Raised, 0.6), Hex("#000000"))}}}
	f[set+"AppIcon-Dark.png"] = encodePNG(Opaque(k.Glyph(Client, 0, 1).Over(darkBack).Image(1024, 1024), Hex("#000000")))
	f[set+"AppIcon-Tinted.png"] = encodePNG(Opaque(k.Mono(Client, 0, Hex("#ffffff"), 1).Image(1024, 1024), Hex("#000000")))
	f[set+"Contents.json"] = contents(map[string]any{"images": []any{
		map[string]any{"filename": "AppIcon.png", "idiom": "universal", "platform": "ios", "size": "1024x1024"},
		map[string]any{"appearances": []any{map[string]any{"appearance": "luminosity", "value": "dark"}}, "filename": "AppIcon-Dark.png", "idiom": "universal", "platform": "ios", "size": "1024x1024"},
		map[string]any{"appearances": []any{map[string]any{"appearance": "luminosity", "value": "tinted"}}, "filename": "AppIcon-Tinted.png", "idiom": "universal", "platform": "ios", "size": "1024x1024"},
	}})

	// Apple Vision Pro: three layers the headset separates in depth.
	stack := ios + "AppIcon.solidimagestack/"
	m := k.Mark(Client, 0)
	vision := []struct {
		name string
		art  Art
	}{
		{"Front", Art{W: Page, H: Page, Layers: glyphAt(m.Front(), 1)}},
		{"Middle", Art{W: Page, H: Page, Layers: glyphAt([]Layer{m.Body}, 1)}},
		{"Back", k.Back(Client)},
	}
	var layers []any
	for _, v := range vision {
		layers = append(layers, map[string]any{"filename": v.name + ".solidimagestacklayer"})
		dir := stack + v.name + ".solidimagestacklayer/"
		f[dir+"Contents.json"] = contents(map[string]any{})
		img := v.art.Image(1024, 1024)
		if v.name == "Back" {
			img = Opaque(img, k.Pal.Night)
		}
		f[dir+"Content.imageset/"+v.name+".png"] = encodePNG(img)
		f[dir+"Content.imageset/Contents.json"] = contents(map[string]any{"images": []any{map[string]any{"filename": v.name + ".png", "idiom": "vision", "scale": "2x"}}})
	}
	f[stack+"Contents.json"] = contents(map[string]any{"layers": layers})

	// The launch screen's mark, over the LaunchBackground colour.
	ls := ios + "LaunchMark.imageset/"
	var lsImages []any
	for _, sc := range []int{1, 2, 3} {
		name := fmt.Sprintf("LaunchMark@%dx.png", sc)
		f[ls+name] = k.Splash(Client, float64(120*sc)).PNG(120*sc, 120*sc)
		lsImages = append(lsImages, map[string]any{"filename": name, "idiom": "universal", "scale": fmt.Sprintf("%dx", sc)})
	}
	f[ls+"Contents.json"] = contents(map[string]any{"images": lsImages})

	// Apple TV: the layered icon (home screen and App Store) and the Top Shelf pictures.
	ba := tv + "App Icon & Top Shelf Image.brandassets/"
	k.tvStack(f, ba+"App Icon.imagestack/", 400, 240, []int{1, 2})
	k.tvStack(f, ba+"App Icon - App Store.imagestack/", 1280, 768, []int{1})
	for _, ts := range []struct {
		name string
		w    int
	}{{"Top Shelf Image", 1920}, {"Top Shelf Image Wide", 2320}} {
		dir := ba + ts.name + ".imageset/"
		var imgs []any
		for _, sc := range []int{1, 2} {
			name := fmt.Sprintf("%s@%dx.png", path.Base(ts.name), sc)
			art := k.TopShelf(float64(ts.w), 720)
			f[dir+name] = encodePNG(Opaque(art.Image(ts.w*sc, 720*sc), k.Pal.Night))
			imgs = append(imgs, map[string]any{"filename": name, "idiom": "tv", "scale": fmt.Sprintf("%dx", sc)})
		}
		f[dir+"Contents.json"] = contents(map[string]any{"images": imgs})
	}
	f[ba+"Contents.json"] = contents(map[string]any{"assets": []any{
		map[string]any{"filename": "App Icon - App Store.imagestack", "idiom": "tv", "role": "primary-app-icon", "size": "1280x768"},
		map[string]any{"filename": "App Icon.imagestack", "idiom": "tv", "role": "primary-app-icon", "size": "400x240"},
		map[string]any{"filename": "Top Shelf Image Wide.imageset", "idiom": "tv", "role": "top-shelf-image-wide", "size": "2320x720"},
		map[string]any{"filename": "Top Shelf Image.imageset", "idiom": "tv", "role": "top-shelf-image", "size": "1920x720"},
	}})

	// Behind the Apple TV's sign-in and connect screens.
	bd := tv + "ConnectBackdrop.imageset/"
	var bdImages []any
	for _, sc := range []int{1, 2} {
		name := fmt.Sprintf("ConnectBackdrop@%dx.jpg", sc)
		f[bd+name] = JPEG(k.Backdrop(Client, 1920, 1080, false).Image(1920*sc, 1080*sc))
		bdImages = append(bdImages, map[string]any{"filename": name, "idiom": "tv", "scale": fmt.Sprintf("%dx", sc)})
	}
	f[bd+"Contents.json"] = contents(map[string]any{"images": bdImages})

	// The marks the apps draw in their own screens.
	f["apple/App/BrandArt.swift"] = SwiftFile([]string{"server", "client"}, []Art{k.Tile(Server, 64), k.Tile(Client, 64)})
}

// tvStack writes a tvOS image stack: back, middle and front layers at each scale.
func (k Kit) tvStack(f Files, dir string, w, h int, scales []int) {
	names := []string{"Back", "Middle", "Front"}
	var layers []any
	for i := len(names) - 1; i >= 0; i-- {
		layers = append(layers, map[string]any{"filename": names[i] + ".imagestacklayer"})
	}
	for _, sc := range scales {
		arts := k.TVLayers(Client, float64(w), float64(h))
		for i, name := range names {
			img := arts[i].Image(w*sc, h*sc)
			if i == 0 {
				img = Opaque(img, k.Pal.Night)
			}
			f[fmt.Sprintf("%s%s.imagestacklayer/Content.imageset/%s@%dx.png", dir, name, name, sc)] = encodePNG(img)
		}
	}
	for _, name := range names {
		var imgs []any
		for _, sc := range scales {
			imgs = append(imgs, map[string]any{"filename": fmt.Sprintf("%s@%dx.png", name, sc), "idiom": "tv", "scale": fmt.Sprintf("%dx", sc)})
		}
		f[dir+name+".imagestacklayer/Contents.json"] = contents(map[string]any{})
		f[dir+name+".imagestacklayer/Content.imageset/Contents.json"] = contents(map[string]any{"images": imgs})
	}
	f[dir+"Contents.json"] = contents(map[string]any{"layers": layers})
}

// ---- Android ----

func (k Kit) android(f Files) {
	res := "android/app/src/main/res/"
	densities := []struct {
		name  string
		scale float64
	}{{"mdpi", 1}, {"hdpi", 1.5}, {"xhdpi", 2}, {"xxhdpi", 3}, {"xxxhdpi", 4}}
	for _, d := range densities {
		px := int(48 * d.scale)
		f[res+"mipmap-"+d.name+"/ic_launcher.png"] = k.Tile(Client, px).PNG(px, px)
		f[res+"mipmap-"+d.name+"/ic_launcher_round.png"] = k.Round(Client, px).PNG(px, px)
	}
	// Adaptive icon (Android 8 on): the launcher cuts its own shape and moves the layers.
	note := "Generated by WittyMediaServer's scripts/genicon (docs/BRAND.md). Do not edit: run scripts/sync-brand.sh."
	// The mark is drawn in gradients, which vector drawables only have from Android 7: the layers
	// that use them live in drawable-v26 (the adaptive icon and the Android 12 splash need 8 and 12).
	f[res+"drawable-v26/ic_launcher_foreground.xml"] = k.Glyph(Client, 0, 1).VectorDrawable(108, 108, note)
	f[res+"drawable/ic_launcher_monochrome.xml"] = k.Mono(Client, 0, Hex("#ffffff"), 1).VectorDrawable(108, 108, note)
	// Gradients in vector drawables need Android 7, and only the adaptive icon (Android 8) uses it.
	f[res+"drawable-v26/ic_launcher_background.xml"] = k.Back(Client).VectorDrawable(108, 108, note)
	adaptive := []byte("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<!-- " + note + " -->\n<adaptive-icon xmlns:android=\"http://schemas.android.com/apk/res/android\">\n    <background android:drawable=\"@drawable/ic_launcher_background\" />\n    <foreground android:drawable=\"@drawable/ic_launcher_foreground\" />\n    <monochrome android:drawable=\"@drawable/ic_launcher_monochrome\" />\n</adaptive-icon>\n")
	f[res+"mipmap-anydpi-v26/ic_launcher.xml"] = adaptive
	f[res+"mipmap-anydpi-v26/ic_launcher_round.xml"] = adaptive
	// The TV launcher's banner: 320×180 dp, at every density (lint asks for each folder).
	for _, d := range densities {
		f[res+"drawable-"+d.name+"/banner.png"] = k.Banner(Client, 320, 180).PNG(int(320*d.scale), int(180*d.scale))
	}
	// Splash: Android 12 on shows splash_icon on the background colour; before that the launch
	// theme's window background is splash_screen (the colour with the mark in the middle).
	// 288 dp is what the Android 12 splash asks for, larger than lint's 200 dp advice for vectors.
	f[res+"drawable-v26/splash_icon.xml"] = []byte(strings.Replace(string(k.Glyph(Client, 0, 1).VectorDrawable(288, 288, note)),
		`xmlns:aapt="http://schemas.android.com/aapt"`, `xmlns:aapt="http://schemas.android.com/aapt"`+"\n    xmlns:tools=\"http://schemas.android.com/tools\"\n    tools:ignore=\"VectorRaster\"", 1))
	// Before Android 12 the launch background is a layer list, back to Android 6, so its mark is a
	// picture at every density rather than a vector with gradients.
	for _, d := range densities {
		px := int(160 * d.scale)
		f[res+"drawable-"+d.name+"/splash_mark.png"] = k.Splash(Client, float64(px)).PNG(px, px)
	}
	f[res+"drawable/splash_screen.xml"] = []byte("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<!-- " + note + " -->\n<layer-list xmlns:android=\"http://schemas.android.com/apk/res/android\">\n    <item android:drawable=\"@color/background\" />\n    <item android:drawable=\"@drawable/splash_mark\" android:gravity=\"center\" android:width=\"160dp\" android:height=\"160dp\" />\n</layer-list>\n")
	// Behind the TV's sign-in and connect screens.
	f[res+"drawable-nodpi/connect_backdrop.jpg"] = JPEG(k.Backdrop(Client, 1920, 1080, false).Image(1920, 1080))
	// The marks the app draws in its own screens.
	f["android/app/src/main/kotlin/com/wittywilla/wittymediaclient/ui/BrandArt.kt"] = KotlinFile("com.wittywilla.wittymediaclient.ui", []string{"Server", "Client"}, []Art{k.Tile(Server, 64), k.Tile(Client, 64)}, 64)
	// For the stores: Google Play's 512 icon and its 1024×500 feature graphic.
	f["android/brand/play-icon-512.png"] = encodePNG(Opaque(k.Full(Client, 512).Image(512, 512), k.Pal.Night))
	f["android/brand/feature-graphic.png"] = encodePNG(Opaque(k.Social(Client, 1024, 500, "Your WittyMediaServer, on your phone and TV.").Image(1024, 500), k.Pal.Night))
}
