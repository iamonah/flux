package l7

import (
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
)

type Backend struct {
	url     *url.URL
	proxy   *httputil.ReverseProxy
	matcher string
	weight  *uint32
}

func NewBackend(url *url.URL, matcher string, weight *uint32) *Backend {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(url)
			req := pr.Out

			req.Header.Del("X-Forwarded-For")
			req.Header.Del("X-Forwarded-Host")
			req.Header.Del("X-Forwarded-Proto")
			req.Header.Del("X-Internal-Secret")
			req.Header.Del("Server")

			clientIP, _, err := net.SplitHostPort(pr.In.RemoteAddr)

			if err == nil {
				req.Header.Set("X-Forwarded-For", clientIP)
			} else {
				req.Header.Set("X-Forwarded-For", pr.In.RemoteAddr)
			}

			req.Header.Set("X-Forwarded-Host", pr.In.Host)

			if pr.In.TLS != nil {
				req.Header.Set("X-Forwarded-Proto", "https")
			} else {
				req.Header.Set("X-Forwarded-Proto", "http")
			}

		},
	}
	return &Backend{
		url:     url,
		proxy:   proxy,
		matcher: matcher,
		weight: weight,
	}
}
func (s *Backend) forward(w http.ResponseWriter, r *http.Request) {
	s.proxy.ServeHTTP(w, r)
}
