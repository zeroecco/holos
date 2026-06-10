package images

import (
	"net"
	"net/http"
	"time"
)

// responseHeaderTimeout bounds the time spent waiting for a mirror to produce
// HTTP headers after the connection is established. Debian's cloud image host
// can be slow to begin large qcow2 responses, so keep this roomier than the
// connect/TLS phase while still failing dead mirrors in minutes, not forever.
var responseHeaderTimeout = 2 * time.Minute

// httpClient is the package-wide client used for image downloads.
// We avoid a total Client.Timeout because cloud images can legitimately
// take a long time to transfer over slow home links. Instead we set
// per-phase timeouts on the Transport so stalled connection setup or
// response headers cannot hang `holos pull` indefinitely.
var httpClient = &http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: responseHeaderTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
	},
}
