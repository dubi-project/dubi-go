package logbase

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

var ErrBackpressure = errors.New("Logbase writer queue is full")
var ErrClosed = errors.New("Logbase writer is closed")

type WriterOptions struct {
	BatchRecords   int           // default 128, at most 1024
	BatchBytes     int           // default 256 KiB, at most 1 MiB
	FlushInterval  time.Duration // default 5ms
	PendingRecords int           // includes in-flight records; default 4096
	PendingBytes   int           // includes in-flight copied input; default 8 MiB
	BatchTimeout   time.Duration // default 45s
}

type delivery struct {
	item AppendItem
	err  error
}
type Receipt struct {
	done   chan struct{}
	result delivery
	raw    json.RawMessage
}

// Wait observes durable confirmation, not local queue acceptance. Cancelling a
// waiter leaves its already accepted record in the writer; Wait can be retried.
func (r *Receipt) Wait(ctx context.Context) (AppendItem, error) {
	select {
	case <-r.done:
		return r.result.item, r.result.err
	default:
	}
	select {
	case <-r.done:
		return r.result.item, r.result.err
	case <-ctx.Done():
		return AppendItem{}, ctx.Err()
	}
}

type Writer struct {
	base    *Logbase
	o       WriterOptions
	mu      sync.Mutex
	queue   []*Receipt
	closed  bool
	pending int
	bytes   int
	wake    chan struct{}
	closing chan struct{}
	done    chan struct{}
}
type WriterStatistics struct {
	PendingRecords int
	PendingBytes   int
	Closed         bool
}

func (l *Logbase) NewWriter(o WriterOptions) (*Writer, error) {
	if o.BatchRecords == 0 {
		o.BatchRecords = 128
	}
	if o.BatchBytes == 0 {
		o.BatchBytes = 256 << 10
	}
	if o.FlushInterval == 0 {
		o.FlushInterval = 5 * time.Millisecond
	}
	if o.PendingRecords == 0 {
		o.PendingRecords = 4096
	}
	if o.PendingBytes == 0 {
		o.PendingBytes = 8 << 20
	}
	if o.BatchTimeout == 0 {
		o.BatchTimeout = 45 * time.Second
	}
	if o.BatchRecords < 1 || o.BatchRecords > MaxRecords || o.BatchBytes < 2 || o.BatchBytes > MaxBytes || o.FlushInterval < 0 || o.FlushInterval > time.Second || o.PendingRecords < o.BatchRecords || o.PendingRecords > 65536 || o.PendingBytes < o.BatchBytes || o.PendingBytes > 64<<20 || o.BatchTimeout < time.Millisecond || o.BatchTimeout > 5*time.Minute {
		return nil, ErrInvalid
	}
	w := &Writer{base: l, o: o, wake: make(chan struct{}, 1), closing: make(chan struct{}), done: make(chan struct{})}
	go w.run()
	return w, nil
}

// Submit copies one JSON object into a bounded local queue. Its Receipt is the
// acknowledgement handle; returning from Submit alone does not mean success.
func (w *Writer) Submit(ctx context.Context, raw json.RawMessage) (*Receipt, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if len(raw) == 0 || len(raw) > w.o.BatchBytes {
		return nil, ErrInvalid
	}
	var dst bytes.Buffer
	if json.Compact(&dst, raw) != nil || dst.Len() == 0 || dst.Bytes()[0] != '{' || dst.Len()+1 > w.o.BatchBytes {
		return nil, ErrInvalid
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil, ErrClosed
	}
	charge := dst.Len() + 1
	if w.pending >= w.o.PendingRecords || w.bytes+charge > w.o.PendingBytes {
		return nil, ErrBackpressure
	}
	r := &Receipt{done: make(chan struct{}), raw: dst.Bytes()}
	w.queue = append(w.queue, r)
	w.pending++
	w.bytes += charge
	select {
	case w.wake <- struct{}{}:
	default:
	}
	return r, nil
}

func (w *Writer) Statistics() WriterStatistics {
	w.mu.Lock()
	defer w.mu.Unlock()
	return WriterStatistics{w.pending, w.bytes, w.closed}
}

// Close stops admission and drains accepted records. If ctx expires, draining
// continues and existing receipts remain usable; a later Close can wait again.
func (w *Writer) Close(ctx context.Context) error {
	w.mu.Lock()
	if !w.closed {
		w.closed = true
		close(w.closing)
	}
	w.mu.Unlock()
	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *Writer) take() []*Receipt {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	size := 0
	for n < len(w.queue) && n < w.o.BatchRecords {
		next := len(w.queue[n].raw) + 1
		if size+next > w.o.BatchBytes {
			break
		}
		size += next
		n++
	}
	batch := append([]*Receipt(nil), w.queue[:n]...)
	copy(w.queue, w.queue[n:])
	clear(w.queue[len(w.queue)-n:])
	w.queue = w.queue[:len(w.queue)-n]
	return batch
}

func (w *Writer) run() {
	defer close(w.done)
	for {
		w.mu.Lock()
		empty := len(w.queue) == 0
		closed := w.closed
		w.mu.Unlock()
		if empty {
			if closed {
				return
			}
			select {
			case <-w.wake:
			case <-w.closing:
			}
			continue
		}
		// Bound collection delay, but skip it while closing or already full.
		deadline := time.NewTimer(w.o.FlushInterval)
	collect:
		for {
			w.mu.Lock()
			full := len(w.queue) >= w.o.BatchRecords
			closed = w.closed
			w.mu.Unlock()
			if full || closed {
				break
			}
			select {
			case <-deadline.C:
				break collect
			case <-w.closing:
				break collect
			case <-w.wake:
			}
		}
		deadline.Stop()
		rows := w.take()
		if len(rows) == 0 {
			continue
		}
		raw := make([]json.RawMessage, len(rows))
		for i, r := range rows {
			raw[i] = r.raw
		}
		var identity [16]byte
		_, err := rand.Read(identity[:])
		var batch *Batch
		var result AppendResult
		if err == nil {
			batch, err = w.base.NewBatch("sdk_"+hex.EncodeToString(identity[:]), raw)
		}
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), w.o.BatchTimeout)
			result, err = batch.Send(ctx)
			cancel()
		}
		items := map[int]AppendItem{}
		for _, item := range result.Items {
			items[item.Index] = item
		}
		w.mu.Lock()
		for i, r := range rows {
			item := items[i]
			item.Index = i
			r.result = delivery{item, err}
			if item.Status == "committed" && item.ID != "" {
				r.result.err = nil
			}
			w.pending--
			w.bytes -= len(r.raw) + 1
			r.raw = nil
			close(r.done)
		}
		w.mu.Unlock()
	}
}
