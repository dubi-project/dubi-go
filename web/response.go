package web

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"mime"
	stdhttp "net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

type jsonBody struct{ value any }
type answerBody struct{ result Res }
type answerEnvelope struct {
	Code int    `json:"code"`
	Time int64  `json:"time"`
	Text string `json:"text,omitempty"`
	Data *Map   `json:"data,omitempty"`
}
type redirectBody struct{ url string }
type fileBody struct {
	path string
	name string
	info os.FileInfo
}
type binaryBody struct {
	data []byte
	name string
}
type streamBody struct {
	reader io.ReadCloser
	size   int64
	name   string
}

var gzipWriterPool = sync.Pool{
	New: func() any { return gzip.NewWriter(io.Discard) },
}

// Response helpers select a body. Output is deferred until all filters return.
func (ctx *Context) Text(value string, codes ...int) {
	ctx.setResponseHeader("Content-Type", "text/plain; charset=utf-8")
	ctx.Body = value
	ctx.setCode(codes)
}

func (ctx *Context) HTML(value string, codes ...int) {
	ctx.setResponseHeader("Content-Type", "text/html; charset=utf-8")
	ctx.Body = value
	ctx.setCode(codes)
}

func (ctx *Context) JSON(value any, codes ...int) {
	ctx.setResponseHeader("Content-Type", "application/json; charset=utf-8")
	ctx.Body = jsonBody{value}
	ctx.setCode(codes)
}

// Answer uses Res.Code() as the business result code, independent of ctx.Code
// (the HTTP status). Data defaults to ctx.Data, which response filters may edit.
func (ctx *Context) Answer(result Res, data ...Map) {
	if result == nil {
		result = dubi.OK
	}
	ctx.Result(result)
	if len(data) > 0 {
		ctx.Data = data[0]
	}
	ctx.setResponseHeader("Content-Type", "application/json; charset=utf-8")
	ctx.Body = answerBody{result}
}

func (ctx *Context) Redirect(url string, codes ...int) {
	ctx.Code = stdhttp.StatusFound
	ctx.setCode(codes)
	ctx.Body = redirectBody{url}
}

// File serves a local file. A name requests attachment/download disposition.
// Native ServeContent handles HEAD, Range, and conditional requests.
func (ctx *Context) File(path string, names ...string) {
	ctx.Body = fileBody{path: path, name: firstName(names)}
}

// fileWithInfo is used by the static-file path, which already performed a
// regular-file check before dispatching the response.
func (ctx *Context) fileWithInfo(path string, info os.FileInfo, names ...string) {
	ctx.Body = fileBody{path: path, name: firstName(names), info: info}
}

func (ctx *Context) Binary(data []byte, names ...string) {
	ctx.Body = binaryBody{data, firstName(names)}
}

// Stream takes ownership of reader until the request ends, including when a
// filter replaces the body. A negative size means unknown length.
func (ctx *Context) Stream(reader io.ReadCloser, size int64, names ...string) {
	if reader == nil {
		ctx.Error(dubi.ErrorResult(fmt.Errorf("web: nil stream reader")))
		return
	}
	ctx.streams = append(ctx.streams, reader)
	ctx.Body = streamBody{reader, size, firstName(names)}
}

func (ctx *Context) setCode(codes []int) {
	if len(codes) > 0 {
		ctx.Code = codes[0]
	}
}

func firstName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func (ctx *Context) closeStreams() {
	for _, reader := range ctx.streams {
		_ = reader.Close()
	}
	ctx.streams = nil
}

func (ctx *Context) marshal(value any) ([]byte, error) {
	return dubi.Marshal("json", value)
}

func (ctx *Context) attachment(name string) {
	if name != "" {
		ctx.Writer.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	}
}

// prepare checks encoders/files before committing any headers. Encoding or
// mapping failure can therefore still be handled by the HTTP Error handler.
func (ctx *Context) prepare() error {
	switch body := ctx.Body.(type) {
	case jsonBody:
		data, err := ctx.marshal(body.value)
		if err != nil {
			return err
		}
		ctx.Body = data
	case answerBody:
		data := ctx.Data
		if body.result.OK() && data != nil && len(ctx.responseVars) > 0 {
			var mapped Map
			var res Res
			if ctx.responsePlan != nil {
				mapped, res = dubi.ApplyMapping(ctx.responsePlan, data)
			} else {
				mapped, res = dubi.Mapping(ctx.responseVars, data)
			}
			if res != nil && res.Fail() {
				return res
			}
			data = mapped
		}
		value := answerEnvelope{Code: body.result.Code(), Time: time.Now().Unix()}
		if text := ctx.String(body.result.Status(), body.result.Args()...); text != "" {
			value.Text = text
		}
		if data != nil {
			value.Data = &data
		}
		encoded, err := ctx.marshal(value)
		if err != nil {
			return err
		}
		ctx.Body = encoded
	case fileBody:
		file, err := os.Open(body.path)
		if err != nil {
			return err
		}
		ctx.streams = append(ctx.streams, file)
		info := body.info
		if info == nil {
			info, err = file.Stat()
			if err != nil {
				return err
			}
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("web: %q is not a regular file", body.path)
		}
		ctx.attachment(body.name)
		ctx.Body = fileContent{file, filepath.Base(body.path), info.ModTime(), fileETag(info)}
	case binaryBody:
		ctx.attachment(body.name)
	case streamBody:
		ctx.attachment(body.name)
		if body.size >= 0 {
			ctx.Writer.Header().Set("Content-Length", strconv.FormatInt(body.size, 10))
		}
		if ctx.Writer.Header().Get("Content-Type") == "" {
			ctx.Writer.Header().Set("Content-Type", "application/octet-stream")
		}
	}
	return nil
}

type fileContent struct {
	file     *os.File
	name     string
	modified time.Time
	etag     string
}

func (p *sitePipeline) finish(ctx *Context) {
	if ctx.output.code != 0 {
		ctx.Code = ctx.output.code
		return
	}
	if err := ctx.prepare(); err != nil {
		ctx.Error(dubi.ErrorResult(err))
		p.handleOutcome(ctx)
		if err := ctx.prepare(); err != nil {
			// A failing error handler must not recurse or commit a partial body.
			ctx.Text(stdhttp.StatusText(stdhttp.StatusInternalServerError), stdhttp.StatusInternalServerError)
		}
	}
	ctx.write()
}

func (ctx *Context) write() {
	if ctx.output.code != 0 {
		return
	}
	if ctx.Code == 0 {
		ctx.Code = stdhttp.StatusOK
		if ctx.Body == nil {
			ctx.Code = stdhttp.StatusNoContent
		}
	}
	if etag := ctx.Writer.Header().Get("ETag"); etag != "" && etagMatches(ctx.Request.Header.Get("If-None-Match"), etag) {
		ctx.Writer.WriteHeader(stdhttp.StatusNotModified)
		ctx.Code = ctx.output.code
		return
	}
	switch body := ctx.Body.(type) {
	case redirectBody:
		stdhttp.Redirect(ctx.Writer, ctx.Request, body.url, ctx.Code)
	case fileContent:
		ctx.Writer.Header().Set("ETag", body.etag)
		if etagMatches(ctx.Request.Header.Get("If-None-Match"), body.etag) {
			ctx.Writer.WriteHeader(stdhttp.StatusNotModified)
			ctx.Code = ctx.output.code
			return
		}
		stdhttp.ServeContent(ctx.Writer, ctx.Request, body.name, body.modified, body.file)
	case binaryBody:
		stdhttp.ServeContent(ctx.Writer, ctx.Request, body.name, time.Time{}, bytes.NewReader(body.data))
	case streamBody:
		ctx.Writer.WriteHeader(ctx.Code)
		if ctx.Request.Method != stdhttp.MethodHead && bodyAllowed(ctx.Code) {
			_, _ = io.Copy(ctx.Writer, body.reader)
		}
	default:
		compress := ctx.Request.Method != stdhttp.MethodHead && bodyAllowed(ctx.Code) && ctx.Body != nil && ctx.shouldCompress()
		if compress {
			ctx.enableCompression()
		}
		ctx.Writer.WriteHeader(ctx.Code)
		if ctx.Request.Method != stdhttp.MethodHead && bodyAllowed(ctx.Code) && ctx.Body != nil {
			ctx.writeValue(body, compress)
		}
	}
	ctx.Code = ctx.output.code
}

func (ctx *Context) writeValue(body any, compress bool) {
	var data []byte
	switch value := body.(type) {
	case []byte:
		data = value
	case string:
		data = []byte(value)
	default:
		_, _ = fmt.Fprint(ctx.Writer, body)
		return
	}
	if compress {
		writer := gzipWriterPool.Get().(*gzip.Writer)
		writer.Reset(ctx.Writer)
		_, _ = writer.Write(data)
		_ = writer.Close()
		writer.Reset(io.Discard)
		gzipWriterPool.Put(writer)
		return
	}
	_, _ = ctx.Writer.Write(data)
}

func (ctx *Context) enableCompression() {
	ctx.Writer.Header().Set("Content-Encoding", "gzip")
	ctx.Writer.Header().Add("Vary", "Accept-Encoding")
	ctx.Writer.Header().Del("Content-Length")
}

func (ctx *Context) shouldCompress() bool {
	if ctx == nil || ctx.Request == nil || ctx.Request.Method == stdhttp.MethodHead || ctx.Setting == nil {
		return false
	}
	if !boolValue(ctx.Setting["compress"]) || ctx.Writer.Header().Get("Content-Encoding") != "" {
		return false
	}
	gzipQuality, wildcardQuality := -1.0, -1.0
	for _, item := range strings.Split(strings.ToLower(ctx.Request.Header.Get("Accept-Encoding")), ",") {
		parts := strings.Split(strings.TrimSpace(item), ";")
		if len(parts) == 0 {
			continue
		}
		quality := 1.0
		for _, parameter := range parts[1:] {
			name, value, ok := strings.Cut(strings.TrimSpace(parameter), "=")
			if ok && strings.TrimSpace(name) == "q" {
				quality, _ = strconv.ParseFloat(strings.TrimSpace(value), 64)
			}
		}
		switch strings.TrimSpace(parts[0]) {
		case "gzip":
			gzipQuality = quality
		case "*":
			wildcardQuality = quality
		}
	}
	if gzipQuality < 0 {
		gzipQuality = wildcardQuality
	}
	if gzipQuality <= 0 {
		return false
	}
	contentType := strings.ToLower(ctx.Writer.Header().Get("Content-Type"))
	return strings.HasPrefix(contentType, "text/") || strings.Contains(contentType, "json") || strings.Contains(contentType, "javascript") || strings.Contains(contentType, "xml")
}

func fileETag(info os.FileInfo) string {
	return fmt.Sprintf("\"%x-%x\"", info.ModTime().UnixNano(), info.Size())
}

func etagMatches(header, etag string) bool {
	for _, value := range strings.Split(header, ",") {
		value = strings.TrimSpace(value)
		if value == "*" || value == etag || strings.TrimPrefix(value, "W/") == etag {
			return true
		}
	}
	return false
}

// CacheControl sets the response cache policy.
func (ctx *Context) CacheControl(value string) {
	ctx.Writer.Header().Set("Cache-Control", value)
}

// ETag sets an explicit response entity tag.
func (ctx *Context) ETag(value string) {
	if value == "" {
		return
	}
	if !strings.HasPrefix(value, "\"") {
		value = fmt.Sprintf("\"%s\"", value)
	}
	ctx.Writer.Header().Set("ETag", value)
}

func bodyAllowed(code int) bool {
	return code >= 200 && code != stdhttp.StatusNoContent && code != stdhttp.StatusNotModified
}
