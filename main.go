package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"time"
	_ "time/tzdata"
)

const interval = 20 * time.Second

type torrent struct {
	Hash     string   `json:"hash,omitempty"`
	Name     string   `json:"name"`
	Size     int64    `json:"size"`
	Speed    int64    `json:"upspeed"`
	State    string   `json:"state"`
	Ratio    float64  `json:"ratio"`
	Progress *float64 `json:"progress,omitempty"`
	Seeding  *int64   `json:"seeding_time,omitempty"`
	Group    string   `json:"group,omitempty"`
}

type snapshot struct {
	At           int64     `json:"at"`
	Speed        int64     `json:"speed"`
	Torrents     []torrent `json:"torrents"`
	TorrentCount *int      `json:"torrent_count,omitempty"`
}

type transfer struct {
	SessionBytes *int64 `json:"up_info_data"`
	Speed        int64  `json:"up_info_speed"`
}

type collector struct {
	url    string
	client *http.Client
	store  *database
	status atomic.Pointer[string]
	details torrentDetails
}

func apiKey() (string, error) {
	data, err := os.ReadFile("/run/panel-secrets/.env")
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			value, ok := strings.CutPrefix(strings.TrimSpace(line), "QBT_API_KEY=")
			if ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value), nil
			}
		}
	}
	return "", errors.New("Falta configurar QBT_API_KEY en el servidor")
}

func (c *collector) get(path, key string, result any) error {
	request, err := http.NewRequest(http.MethodGet, c.url+"/api/v2/"+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+key)
	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("%s: qBittorrent no responde", path)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: qBittorrent devolvió HTTP %d", path, response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<20)).Decode(result); err != nil {
		return fmt.Errorf("%s: respuesta de qBittorrent inválida", path)
	}
	return nil
}

func torrentGroup(state string) string {
	switch state {
	case "uploading", "stalledUP", "forcedUP", "queuedUP", "checkingUP":
		return "sharing"
	case "error", "missingFiles", "stoppedUP", "stoppedDL", "pausedUP", "pausedDL":
		return "stopped"
	case "downloading", "stalledDL", "forcedDL", "queuedDL", "checkingDL", "metaDL", "forcedMetaDL", "allocating":
		return "downloading"
	default:
		return ""
	}
}

func (c *collector) poll() error {
	key, err := apiKey()
	if err != nil {
		return err
	}
	var traffic transfer
	if err := c.get("transfer/info", key, &traffic); err != nil {
		return err
	}
	at := time.Now().Unix()
	if traffic.SessionBytes == nil || *traffic.SessionBytes < 0 {
		return errors.New("qBittorrent no devolvió el contador de subida")
	}
	var torrents []torrent
	if err := c.get("torrents/info", key, &torrents); err != nil {
		return err
	}
	if torrents == nil {
		return errors.New("qBittorrent no devolvió una lista de torrents")
	}
	visible := make([]torrent, 0, len(torrents))
	for _, item := range torrents {
		item.Group = torrentGroup(item.State)
		if item.Group != "" {
			visible = append(visible, item)
		}
	}
	sort.SliceStable(visible, func(i, j int) bool {
		if visible[i].Speed != visible[j].Speed {
			return visible[i].Speed > visible[j].Speed
		}
		return strings.ToLower(visible[i].Name) < strings.ToLower(visible[j].Name)
	})
	count := len(torrents)
	state := snapshot{At: at, Speed: traffic.Speed, Torrents: visible, TorrentCount: &count}
	if err := c.store.save(state, *traffic.SessionBytes); err != nil {
		return err
	}
	c.collectDetail(key, visible)
	return nil
}

func (c *collector) run() {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	var previous string
	for {
		message := ""
		if err := c.poll(); err != nil {
			message = err.Error()
		}
		if message != previous {
			if message == "" {
				log.Print("Recogida de datos restablecida")
			} else {
				log.Print(message)
			}
		}
		c.status.Store(&message)
		previous = message
		<-tick.C
	}
}

func main() {
	db, err := openDB("/data/panel.db")
	if err != nil {
		log.Fatal(err)
	}
	location, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		log.Fatal(err)
	}
	c := &collector{
		url: strings.TrimRight(os.Getenv("QBT_URL"), "/"),
		client: &http.Client{
			Timeout:       8 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		store: db,
	}
	go c.run()
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServer(http.Dir("/usr/share/seed-panel")))
	mux.HandleFunc("GET /api/torrent-detail", c.serveDetail)
	mux.HandleFunc("GET /api/panel", func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().In(location)
		day, week, samples, state, err := db.read(now)
		if err != nil {
			log.Print(err)
			http.Error(w, "No se pudo leer el historial", http.StatusInternalServerError)
			return
		}
		status := ""
		if message := c.status.Load(); message != nil {
			status = *message
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		err = json.NewEncoder(w).Encode(struct {
			State  snapshot `json:"snapshot"`
			Today  []bucket `json:"today"`
			Week   []bucket `json:"week"`
			Status string   `json:"status"`
			Stale  bool     `json:"stale"`
		}{state, aggregate(day, samples), aggregate(week, samples), status, state.At == 0 || now.Unix()-state.At > 150 || status != ""})
		if err != nil {
			log.Print(err)
		}
	})
	server := &http.Server{
		Addr: ":8080",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; base-uri 'none'")
			mux.ServeHTTP(w, r)
		}),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
	}
	log.Print("Panel escuchando en :8080")
	log.Fatal(server.ListenAndServe())
}
