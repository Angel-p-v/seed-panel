package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type detail struct {
	Wasted *int64 `json:"total_wasted"`
	At int64 `json:"at"`
}

const detailInterval = time.Minute

type torrentDetails struct {
	mu          sync.Mutex
	active      map[string]detail
	pending     []string
	lastAttempt time.Time
}

func (c *collector) serveDetail(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("hash")
	c.details.mu.Lock()
	item, exists := c.details.active[hash]
	if exists && time.Now().Unix()-item.At >= int64(detailInterval/time.Second) {
		queued := false
		for _, request := range c.details.pending {
			if request == hash {
				queued = true
				break
			}
		}
		if !queued && len(c.details.pending) < 16 {
			c.details.pending = append(c.details.pending, hash)
		}
	}
	c.details.mu.Unlock()
	if !exists {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(item)
}

func (c *collector) collectDetail(key string, torrents []torrent) {
	c.details.mu.Lock()
	active := make(map[string]detail, len(torrents))
	for _, item := range torrents {
		if item.Hash != "" {
			active[item.Hash] = c.details.active[item.Hash]
		}
	}
	c.details.active = active
	now := time.Now()
	if now.Sub(c.details.lastAttempt) < detailInterval {
		c.details.mu.Unlock()
		return
	}
	hash := ""
	for len(c.details.pending) > 0 {
		next := c.details.pending[0]
		c.details.pending = c.details.pending[1:]
		if item, exists := active[next]; exists && now.Unix()-item.At >= int64(detailInterval/time.Second) {
			hash = next
			c.details.lastAttempt = now
			break
		}
	}
	c.details.mu.Unlock()
	if hash == "" {
		return
	}
	var item detail
	if err := c.get("torrents/properties?hash="+url.QueryEscape(hash), key, &item); err != nil {
		return
	}
	item.At = time.Now().Unix()
	c.details.mu.Lock()
	c.details.active[hash] = item
	c.details.mu.Unlock()
}
