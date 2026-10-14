package web

import (
	"bufio"
	"io"
	"net"
	stdhttp "net/http"
	"sync"
)

var responseWriterPool = sync.Pool{
	New: func() any { return &responseWriter{} },
}

// responseWriter tracks output committed by both the framework and native
// HTTP handlers. Unwrap retains ResponseController support.
type responseWriter struct {
	stdhttp.ResponseWriter
	code int
}

func (w *responseWriter) WriteHeader(code int) {
	if w.code != 0 {
		return
	}
	if code >= 100 && code < 200 && code != stdhttp.StatusSwitchingProtocols {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseWriter) Write(data []byte) (int, error) {
	if w.code == 0 {
		w.WriteHeader(stdhttp.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

func (w *responseWriter) Unwrap() stdhttp.ResponseWriter { return w.ResponseWriter }

func (w *responseWriter) Flush() {
	if w.code == 0 {
		w.WriteHeader(stdhttp.StatusOK)
	}
	_ = stdhttp.NewResponseController(w.ResponseWriter).Flush()
}

func (w *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return stdhttp.NewResponseController(w.ResponseWriter).Hijack()
}

func (w *responseWriter) ReadFrom(reader io.Reader) (int64, error) {
	if w.code == 0 {
		w.WriteHeader(stdhttp.StatusOK)
	}
	if target, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return target.ReadFrom(reader)
	}
	return io.Copy(w.ResponseWriter, reader)
}
