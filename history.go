package main

/*
#cgo pkg-config: sqlite3
#include <sqlite3.h>
#include <stdlib.h>

static int bind_snapshot(sqlite3_stmt *query, const char *data, int size) {
	return sqlite3_bind_text(query, 2, data, size, SQLITE_TRANSIENT);
}
*/
import "C"

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
	"unsafe"
)

type database struct {
	handle *C.sqlite3
	mu     sync.Mutex
}

type sample struct {
	at      int64
	bytes   int64
	uploads []torrentUpload
}

type torrentUpload struct {
	Hash  string `json:"hash"`
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

type bucket struct {
	Label   string          `json:"label"`
	Start   int64           `json:"start"`
	End     int64           `json:"end"`
	Bytes   int64           `json:"bytes"`
	Seconds int64           `json:"seconds"`
	Uploads []torrentUpload `json:"uploads,omitempty"`
}

func openDB(path string) (*database, error) {
	var handle *C.sqlite3
	filename := C.CString(path)
	defer C.free(unsafe.Pointer(filename))
	code := C.sqlite3_open(filename, &handle)
	db := &database{handle: handle}
	if code != C.SQLITE_OK {
		err := db.err()
		C.sqlite3_close(handle)
		return nil, err
	}
	if err := db.exec(`
		CREATE TABLE IF NOT EXISTS current_state (id INTEGER PRIMARY KEY CHECK(id = 1), snapshot TEXT NOT NULL);
		CREATE TABLE IF NOT EXISTS samples (at INTEGER PRIMARY KEY, bytes INTEGER NOT NULL);
		CREATE TABLE IF NOT EXISTS torrent_uploads (at INTEGER PRIMARY KEY, detail TEXT NOT NULL);
	`); err != nil {
		C.sqlite3_close(handle)
		return nil, err
	}
	return db, nil
}

func (db *database) err() error {
	return errors.New(C.GoString(C.sqlite3_errmsg(db.handle)))
}

func (db *database) exec(sql string) error {
	query := C.CString(sql)
	defer C.free(unsafe.Pointer(query))
	if C.sqlite3_exec(db.handle, query, nil, nil, nil) != C.SQLITE_OK {
		return db.err()
	}
	return nil
}

func (db *database) prepare(sql string) (*C.sqlite3_stmt, error) {
	query := C.CString(sql)
	defer C.free(unsafe.Pointer(query))
	var statement *C.sqlite3_stmt
	if C.sqlite3_prepare_v2(db.handle, query, -1, &statement, nil) != C.SQLITE_OK {
		return nil, db.err()
	}
	return statement, nil
}

func (db *database) save(current snapshot, counter int64, uploads []torrentUpload) error {
	data, err := json.Marshal(current)
	if err != nil {
		return err
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	if err := db.exec("BEGIN"); err != nil {
		return err
	}
	defer db.exec("ROLLBACK")
	query, err := db.prepare("INSERT OR REPLACE INTO current_state(id, snapshot) VALUES(?1, ?2)")
	if err != nil {
		return err
	}
	defer C.sqlite3_finalize(query)
	C.sqlite3_bind_int(query, 1, 1)
	content := C.CString(string(data))
	defer C.free(unsafe.Pointer(content))
	if C.bind_snapshot(query, content, C.int(len(data))) != C.SQLITE_OK || C.sqlite3_step(query) != C.SQLITE_DONE {
		return db.err()
	}
	keepFrom := current.At - int64((15 * 24 * time.Hour).Seconds())
	if err := db.exec(fmt.Sprintf("INSERT OR REPLACE INTO samples VALUES(%d, %d); DELETE FROM samples WHERE at < %d", current.At, counter, keepFrom)); err != nil {
		return err
	}
	if err := db.exec(fmt.Sprintf("DELETE FROM torrent_uploads WHERE at < %d OR at = %d", keepFrom, current.At)); err != nil {
		return err
	}
	if len(uploads) > 0 {
		detail, err := json.Marshal(uploads)
		if err != nil {
			return err
		}
		query, err := db.prepare("INSERT OR REPLACE INTO torrent_uploads VALUES(?1, ?2)")
		if err != nil {
			return err
		}
		defer C.sqlite3_finalize(query)
		content := C.CString(string(detail))
		defer C.free(unsafe.Pointer(content))
		C.sqlite3_bind_int64(query, 1, C.sqlite3_int64(current.At))
		if C.bind_snapshot(query, content, C.int(len(detail))) != C.SQLITE_OK || C.sqlite3_step(query) != C.SQLITE_DONE {
			return db.err()
		}
	}
	return db.exec("COMMIT")
}

func (db *database) read(now time.Time) (day, week []bucket, samples []sample, current snapshot, err error) {
	day, week = periods(now)
	db.mu.Lock()
	defer db.mu.Unlock()
	current, err = db.latest()
	if err != nil {
		return
	}
	start := week[0].Start - 150
	query, err := db.prepare(`SELECT samples.at, bytes, detail FROM samples
		LEFT JOIN torrent_uploads USING (at) WHERE samples.at BETWEEN ?1 AND ?2 ORDER BY samples.at`)
	if err != nil {
		return
	}
	defer C.sqlite3_finalize(query)
	C.sqlite3_bind_int64(query, 1, C.sqlite3_int64(start))
	C.sqlite3_bind_int64(query, 2, C.sqlite3_int64(now.Unix()))
	for {
		code := C.sqlite3_step(query)
		switch code {
		case C.SQLITE_DONE:
			return
		case C.SQLITE_ROW:
			item := sample{at: int64(C.sqlite3_column_int64(query, 0)), bytes: int64(C.sqlite3_column_int64(query, 1))}
			if C.sqlite3_column_type(query, 2) != C.SQLITE_NULL {
				data := C.GoBytes(unsafe.Pointer(C.sqlite3_column_text(query, 2)), C.sqlite3_column_bytes(query, 2))
				if err = json.Unmarshal(data, &item.uploads); err != nil {
					return
				}
			}
			samples = append(samples, item)
		default:
			err = db.err()
			return
		}
	}
}

func (db *database) latest() (snapshot, error) {
	current := snapshot{Torrents: []torrent{}}
	query, err := db.prepare("SELECT snapshot FROM current_state WHERE id = 1")
	if err != nil {
		return current, err
	}
	defer C.sqlite3_finalize(query)
	switch C.sqlite3_step(query) {
	case C.SQLITE_DONE:
		return current, nil
	case C.SQLITE_ROW:
		data := C.GoBytes(unsafe.Pointer(C.sqlite3_column_text(query, 0)), C.sqlite3_column_bytes(query, 0))
		err := json.Unmarshal(data, &current)
		return current, err
	default:
		return current, db.err()
	}
}

func periods(now time.Time) (day, week []bucket) {
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	tomorrow := midnight.AddDate(0, 0, 1)
	for hour := midnight; hour.Before(tomorrow); hour = hour.Add(time.Hour) {
		label := hour.Format("15:04")
		if hour.Add(-time.Hour).Format("15:04") == label {
			label += " (segunda)"
		}
		day = append(day, bucket{Label: label, Start: hour.Unix(), End: hour.Add(time.Hour).Unix()})
	}
	monday := midnight.AddDate(0, 0, -(int(now.Weekday())+6)%7)
	labels := []string{"Lun", "Mar", "Mié", "Jue", "Vie", "Sáb", "Dom"}
	for index, label := range labels {
		start := monday.AddDate(0, 0, index)
		week = append(week, bucket{Label: label, Start: start.Unix(), End: start.AddDate(0, 0, 1).Unix()})
	}
	return
}

func aggregate(buckets []bucket, samples []sample) []bucket {
	totals := make([]map[string]torrentUpload, len(buckets))
	index := 0
	for i := 1; i < len(samples); i++ {
		first, second := samples[i-1], samples[i]
		duration := second.at - first.at
		delta := second.bytes - first.bytes
		if duration <= 0 || duration > 90 || delta < 0 {
			continue
		}
		for index < len(buckets) && buckets[index].End <= first.at {
			index++
		}
		for j := index; j < len(buckets) && buckets[j].Start < second.at; j++ {
			bar := &buckets[j]
			start := max(bar.Start, first.at) - first.at
			end := min(bar.End, second.at) - first.at
			// Split the observed increment proportionally across a calendar boundary.
			part := func(bytes, seconds int64) int64 {
				return (bytes/duration)*seconds + (bytes%duration)*seconds/duration
			}
			bar.Bytes += part(delta, end) - part(delta, start)
			bar.Seconds += end - start
			for _, upload := range second.uploads {
				bytes := part(upload.Bytes, end) - part(upload.Bytes, start)
				if bytes == 0 {
					continue
				}
				if totals[j] == nil {
					totals[j] = make(map[string]torrentUpload)
				}
				upload.Bytes = totals[j][upload.Hash].Bytes + bytes
				totals[j][upload.Hash] = upload
			}
		}
	}
	for i, uploads := range totals {
		for _, upload := range uploads {
			buckets[i].Uploads = append(buckets[i].Uploads, upload)
		}
		sort.Slice(buckets[i].Uploads, func(a, b int) bool {
			first, second := buckets[i].Uploads[a], buckets[i].Uploads[b]
			if first.Bytes != second.Bytes {
				return first.Bytes > second.Bytes
			}
			return first.Hash < second.Hash
		})
	}
	return buckets
}
