package proxy

import (
	"compress/gzip"
	"compress/zlib"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/vincentsch/tabslip/internal/apis"
)

type digest struct {
	mu   sync.Mutex
	h    hash.Hash
	size int64
}

func newDigest() *digest { return &digest{h: sha256.New()} }
func (d *digest) Write(b []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.h.Write(b)
	d.size += int64(len(b))
}
func (d *digest) Snapshot() (string, int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return hex.EncodeToString(d.h.Sum(nil)), d.size
}
func (d *digest) Sum() string { s, _ := d.Snapshot(); return s }

type requestTap struct {
	io.ReadCloser
	digest  *digest
	content *boundedBuffer
}

func (t *requestTap) Read(b []byte) (int, error) {
	n, e := t.ReadCloser.Read(b)
	t.digest.Write(b[:n])
	if t.content != nil {
		t.content.Write(b[:n])
	}
	return n, e
}
func appendBounded(dst *[]byte, truncated *bool, b []byte) {
	n := apis.Limit - len(*dst)
	if len(b) > n {
		*truncated = true
		b = b[:n]
	}
	*dst = append(*dst, b...)
}

type boundedBuffer struct {
	mu        sync.Mutex
	data      []byte
	truncated bool
}

func (b *boundedBuffer) Write(p []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	appendBounded(&b.data, &b.truncated, p)
}
func (b *boundedBuffer) Snapshot() ([]byte, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.data...), b.truncated
}

type responseTap struct {
	io.ReadCloser
	digest     *digest
	first      *time.Time
	pipe       *io.PipeWriter
	done       chan struct{}
	eof        bool
	readFailed bool
	closed     bool
}

func newResponseTap(body io.ReadCloser, encoding string, capture *apis.Capture, stream bool) *responseTap {
	pr, pw := io.Pipe()
	t := &responseTap{ReadCloser: body, digest: newDigest(), pipe: pw, done: make(chan struct{})}
	go func() {
		defer close(t.done)
		defer pr.Close()
		defer io.Copy(io.Discard, pr)
		var src io.Reader = pr
		var closer io.Closer
		var err error
		switch strings.ToLower(strings.TrimSpace(encoding)) {
		case "", "identity":
		case "gzip":
			var z *gzip.Reader
			z, err = gzip.NewReader(pr)
			src = z
			closer = z
		case "deflate":
			var z io.ReadCloser
			z, err = zlib.NewReader(pr)
			src = z
			closer = z
		case "br":
			src = brotli.NewReader(pr)
		default:
			apis.Unavailable(capture.Receipt)
			return
		}
		if err != nil {
			apis.Unavailable(capture.Receipt)
			return
		}
		if closer != nil {
			defer closer.Close()
		}
		capture.Parse(src, stream)
		// Drain the decoder to catch checksum errors, then drain raw bytes even on failure.
		if _, err = io.Copy(io.Discard, src); err != nil {
			apis.Unavailable(capture.Receipt)
		}
	}()
	return t
}
func (t *responseTap) Read(b []byte) (int, error) {
	n, e := t.ReadCloser.Read(b)
	if n > 0 {
		if t.first == nil {
			now := time.Now()
			t.first = &now
		}
		t.digest.Write(b[:n])
		_, _ = t.pipe.Write(b[:n])
	}
	if e == io.EOF {
		t.eof = true
	}
	if e != nil && e != io.EOF {
		t.readFailed = true
	}
	return n, e
}
func (t *responseTap) Close() error {
	if !t.closed {
		t.closed = true
		err := t.ReadCloser.Close()
		t.pipe.Close()
		<-t.done
		return err
	}
	return nil
}
