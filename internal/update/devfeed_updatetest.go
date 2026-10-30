//go:build updatetest

package update

import (
	"net"
	"net/http"
	"net/url"
	"os"
)

// Development builds only (go build -tags updatetest): test the updater
// against a fake release on this machine, without publishing anything.
//
//	WMS_UPDATE_FEED  address of a releases list in GitHub's format
//	WMS_UPDATE_KEY   base64 public key of a throwaway signing key
//
// Downloads may then also come from this machine (loopback addresses).
// Release builds never include this file.
func init() {
	if k := os.Getenv("WMS_UPDATE_KEY"); k != "" {
		devKeys = append(devKeys, k)
	}
	if f := os.Getenv("WMS_UPDATE_FEED"); f != "" {
		devFeedURL = f
		devAllowURL = func(u *url.URL) bool {
			ip := net.ParseIP(u.Hostname())
			return (u.Scheme == "http" || u.Scheme == "https") && (u.Hostname() == "localhost" || ip != nil && ip.IsLoopback())
		}
		devTransport = http.DefaultTransport
	}
}
