module github.com/wittywilla/wittymediaserver

go 1.26.0

require (
	fyne.io/systray v1.12.2
	github.com/wittywilla/wittybluray v0.1.0
	github.com/wittywilla/wittydisc v0.1.0
	github.com/wittywilla/wittydvd v0.1.0
	github.com/wittywilla/wittyintegratedfilebrowser v0.1.0
	golang.org/x/crypto v0.58.0
	golang.org/x/sys v0.49.0
	modernc.org/sqlite v1.61.0
)

require (
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/godbus/dbus/v5 v5.2.4 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	modernc.org/libc v1.79.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)

replace github.com/wittywilla/wittydvd => ./modules/wittydvd

replace github.com/wittywilla/wittydisc => ./modules/wittydisc

replace github.com/wittywilla/wittybluray => ./modules/wittybluray

// Not released yet: built from this repository's copy.
replace github.com/wittywilla/wittyintegratedfilebrowser => ./modules/wittyintegratedfilebrowser
