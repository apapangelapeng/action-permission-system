package api

// Server-sent events for the dashboard: instead of polling every few seconds,
// signed-in dashboards hold one SSE stream and get nudged when anything
// changes. The event carries no data — clients refetch through the normal
// endpoints — so there is nothing to keep consistent and polling remains a
// drop-in fallback when the stream is down.

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/apapangelapeng/action-permission-system/internal/store"
)

type Broadcaster struct {
	mu   sync.Mutex
	subs map[chan struct{}]struct{}
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{subs: map[chan struct{}]struct{}{}}
}

// Publish nudges every connected dashboard. Non-blocking: a slow consumer
// with a full buffer just misses a nudge — its next poll catches it up.
func (b *Broadcaster) Publish() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (b *Broadcaster) subscribe() (chan struct{}, func()) {
	ch := make(chan struct{}, 4)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}
}

// events backs GET /v1/events. EventSource cannot send an Authorization
// header, so the session token arrives as a query parameter for this one
// endpoint — validated against the same sessions table.
func (h *handlers) events(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		writeError(w, http.StatusUnauthorized, "missing token query parameter")
		return
	}
	if _, err := h.st.UserBySessionToken(r.Context(), token); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "invalid or expired session")
		} else {
			writeError(w, http.StatusInternalServerError, "auth lookup failed")
		}
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "event: hello\ndata: {}\n\n")
	flusher.Flush()

	ch, cancel := h.bc.subscribe()
	defer cancel()
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			fmt.Fprint(w, "event: changed\ndata: {}\n\n")
			flusher.Flush()
		case <-heartbeat.C: // keeps proxies and browsers from closing an idle stream
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}
