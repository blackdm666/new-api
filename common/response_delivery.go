package common

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
)

type responseDeliveryKey struct{}

// ResponseDelivery contains transport evidence, not a provider outcome. A
// successful Write alone may only fill net/http's buffer; Flush must succeed.
type ResponseDelivery struct {
	mu       sync.Mutex
	written  int64
	flushed  int64
	expected int64
	failed   bool
}

// ResponseDeliverySnapshot deliberately contains no response content.
type ResponseDeliverySnapshot struct {
	BodyComplete bool
	Flushed      bool
	Failed       bool
}

func GetResponseDelivery(ctx context.Context) ResponseDeliverySnapshot {
	if ctx == nil {
		return ResponseDeliverySnapshot{}
	}
	d, _ := ctx.Value(responseDeliveryKey{}).(*ResponseDelivery)
	if d == nil {
		return ResponseDeliverySnapshot{}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	flushed := !d.failed && d.written > 0 && d.flushed == d.written
	return ResponseDeliverySnapshot{
		BodyComplete: flushed && d.expected >= 0 && d.written == d.expected,
		Flushed:      flushed,
		Failed:       d.failed,
	}
}

// MarkResponseDeliveryAborted accounts for helpers that skip a write when the
// client is already gone. Otherwise an earlier flushed chunk could be mistaken
// for a terminal event that the adaptor parsed but never forwarded.
func MarkResponseDeliveryAborted(ctx context.Context) {
	if ctx == nil {
		return
	}
	if d, _ := ctx.Value(responseDeliveryKey{}).(*ResponseDelivery); d != nil {
		d.mu.Lock()
		d.failed = true
		d.mu.Unlock()
	}
}

// TrackResponseDelivery must wrap the HTTP handler outside Gin: Gin's Flush
// discards errors, so observing only gin.ResponseWriter cannot prove delivery.
// Wrapping here also preserves any compression/other middleware's own Flush.
func TrackResponseDelivery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d := &ResponseDelivery{expected: -1}
		next.ServeHTTP(&deliveryWriter{ResponseWriter: w, delivery: d},
			r.WithContext(context.WithValue(r.Context(), responseDeliveryKey{}, d)))
	})
}

type deliveryWriter struct {
	http.ResponseWriter
	delivery *ResponseDelivery
	header   bool
}

func (w *deliveryWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *deliveryWriter) WriteHeader(status int) {
	if !w.header && status >= 200 {
		w.header = true
		expected, err := strconv.ParseInt(w.Header().Get("Content-Length"), 10, 64)
		if err == nil && expected >= 0 {
			w.delivery.mu.Lock()
			w.delivery.expected = expected
			w.delivery.mu.Unlock()
		}
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *deliveryWriter) Write(p []byte) (int, error) {
	if !w.header {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	w.delivery.mu.Lock()
	w.delivery.written += int64(n)
	w.delivery.failed = w.delivery.failed || err != nil
	w.delivery.mu.Unlock()
	return n, err
}

func (w *deliveryWriter) FlushError() error {
	if !w.header {
		w.WriteHeader(http.StatusOK)
	}
	// A panic must not leave the previous flush looking like final delivery.
	w.delivery.mu.Lock()
	w.delivery.flushed = -1
	w.delivery.mu.Unlock()
	err := http.NewResponseController(w.ResponseWriter).Flush()
	w.delivery.mu.Lock()
	w.delivery.failed = w.delivery.failed || err != nil
	if err == nil {
		w.delivery.flushed = w.delivery.written
	}
	w.delivery.mu.Unlock()
	return err
}

func (w *deliveryWriter) Flush() { _ = w.FlushError() }

func (w *deliveryWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

func (w *deliveryWriter) CloseNotify() <-chan bool {
	return w.ResponseWriter.(http.CloseNotifier).CloseNotify()
}

func (w *deliveryWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := w.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}
