// Package httpstream connects streaming net/http handlers to the application's
// buffered HTTP server while retaining status information for request logs.
package httpstream

import (
	"bufio"
	"net"
	"net/http"

	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
)

// Writer sends bytes directly to the connection, without filling the framework's
// response buffer. Use it for reverse proxies and http.ServeContent.
type Writer struct {
	response *ghttp.Response
	raw      http.ResponseWriter
}

func New(response *ghttp.Response) *Writer {
	return &Writer{response: response, raw: response.RawWriter()}
}

func (w *Writer) Header() http.Header { return w.raw.Header() }

func (w *Writer) WriteHeader(status int) {
	if w.response.IsHeaderWrote() {
		return
	}
	w.response.Status = status
	w.raw.WriteHeader(status)
}

func (w *Writer) Write(p []byte) (int, error) {
	if !w.response.IsHeaderWrote() {
		w.WriteHeader(http.StatusOK)
	}
	return w.raw.Write(p)
}

// Flush commits headers and existing network buffers only. The framework's
// BufferWriter.Flush also invents a default status body, which is inappropriate
// when a reverse proxy flushes response headers before its first body chunk.
func (w *Writer) Flush() {
	if !w.response.IsHeaderWrote() {
		w.WriteHeader(http.StatusOK)
	}
	if f, ok := w.raw.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *Writer) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.raw).Hijack()
}

func (w *Writer) Unwrap() http.ResponseWriter { return w.raw }
