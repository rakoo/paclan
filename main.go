package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path"
	"sync"
	"time"
)

const (
	HTTP_PORT         = `15678`
	MULTICAST_ADDRESS = `224.3.45.67:15679`
	PKG_CACHE_DIR     = `/var/cache/pacman/pkg`
	DB_DIR            = `/var/lib/pacman/sync`
	TTL               = 1 * time.Hour
	MULTICAST_DELAY   = 10 * time.Minute
)

var (
	peers    = newPeerMap()
	seenTags = newTagMap()
)

type peerMap struct {
	sync.Mutex
	peers  map[string]struct{}
	expire chan string
}

func newPeerMap() peerMap {
	p := peerMap{peers: make(map[string]struct{})}
	go p.expireLoop()

	return p
}

func (p peerMap) expireLoop() {
	for {
		select {
		case peer := <-p.expire:
			p.Lock()
			delete(p.peers, peer)
			p.Unlock()
		}
	}
}

func (p peerMap) Add(peer string) {
	p.Lock()
	p.peers[peer] = struct{}{}
	time.AfterFunc(TTL, func() { p.expire <- peer })
	p.Unlock()
}

func (p peerMap) GetRandomOrder() []string {
	p.Lock()

	peers := make([]string, len(p.peers))
	for peer := range p.peers {
		max := big.NewInt(int64(len(peers)))
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			log.Printf("Couldn't get random int: %s\n", err)
			continue
		}
		peers[idx.Int64()] = peer
	}

	p.Unlock()

	return peers
}

type TagMap struct {
	sync.Mutex
	tags   map[string]struct{}
	expire chan string
}

func newTagMap() TagMap {
	t := TagMap{
		tags:   make(map[string]struct{}),
		expire: make(chan string),
	}

	go func() {
		for {
			select {
			case tag := <-t.expire:
				delete(t.tags, tag)
			}
		}
	}()

	return t
}

func (t TagMap) Mark(tag string) {
	t.Lock()
	t.tags[tag] = struct{}{}
	time.AfterFunc(TTL, func() { t.expire <- tag })
	t.Unlock()
}

func (t TagMap) IsNew(tag string) bool {
	t.Lock()
	_, ok := t.tags[tag]
	t.Unlock()

	return !ok
}

func main() {
	go serveMulticast()
	go serveHttp()
	select {}
}

func serveHttp() {
	http.Handle("/", http.HandlerFunc(handle))

	log.Println("Serving from", HTTP_PORT)
	err := http.ListenAndServe(":"+HTTP_PORT, nil)
	if err != nil {
		log.Fatal(err)
	}
}

func handle(w http.ResponseWriter, r *http.Request) {
	addr, err := net.ResolveTCPAddr("tcp", r.RemoteAddr)
	if err != nil {
		log.Printf("Error serving %s: %s\n", r.RemoteAddr, err)
		return
	}

	if addr.IP.IsLoopback() {
		handleLocal(w, r)
	} else {
		handleRemote(w, r)
	}
}

func handleLocal(w http.ResponseWriter, r *http.Request) {
	for _, peer := range peers.GetRandomOrder() {
		newUrl := *r.URL
		newUrl.Host = peer
		newUrl.Scheme = "http"

		resp, err := http.Head(newUrl.String())
		if err == nil {
			if r.Method == "HEAD" {
				w.WriteHeader(resp.StatusCode)
				return
			} else if r.Method == "GET" && resp.StatusCode == http.StatusOK {
				http.Redirect(w, r, newUrl.String(), http.StatusFound)
				return
			}
		}
	}

	w.WriteHeader(http.StatusNotFound)
}

func handleRemote(w http.ResponseWriter, r *http.Request) {
	fpath := path.Join(PKG_CACHE_DIR, path.Base(r.URL.Path))
	_, err := os.Stat(fpath)

	if err == nil {
		if r.Method == "HEAD" {
			w.WriteHeader(http.StatusOK)
		} else if r.Method == "GET" {
			http.ServeFile(w, r, fpath)
		}
		return
	}

	w.WriteHeader(http.StatusNotFound)
}

type Announce struct {
	Port string `json:"port"`
	Tag  string `json:"tag"`
}

type multicaster struct {
	conn *net.UDPConn
	addr *net.UDPAddr
}

func serveMulticast() {
	addr, err := net.ResolveUDPAddr("udp4", MULTICAST_ADDRESS)
	if err != nil {
		return
	}

	conn, err := net.ListenMulticastUDP("udp4", nil, addr)
	if err != nil {
		return
	}

	mc := multicaster{conn: conn, addr: addr}
	mc.run()
}

func (mc multicaster) run() {
	go mc.listenLoop()

	mc.sendAnnounce()
	for {
		<-time.After(MULTICAST_DELAY)
		mc.sendAnnounce()
	}
}

func (mc multicaster) sendAnnounce() {
	tagRaw := make([]byte, 8)
	_, err := rand.Read(tagRaw)
	if err != nil {
		log.Printf("Couldn't create tag: %s\n", err)
		return
	}

	tag := hex.EncodeToString(tagRaw)
	mc.sendAnnounceWithTag(tag)
}

func (mc multicaster) sendAnnounceWithTag(tag string) {
	msg := Announce{Port: HTTP_PORT, Tag: tag}
	raw, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Couldn't serialize announce: %s\n", err)
		return
	}

	mc.conn.WriteToUDP(raw, mc.addr)
	seenTags.Mark(msg.Tag)
}

func (mc multicaster) listenLoop() {
	for {
		packet := make([]byte, 256)
		_, from, err := mc.conn.ReadFromUDP(packet)
		if err != nil {
			log.Printf("Error reading from %s: %s\n", from, err)
			continue
		}

		var msg Announce
		err = json.NewDecoder(bytes.NewReader(packet)).Decode(&msg)
		if err != nil {
			log.Printf("Couldn't unserialize announce [%s]: %s\n", packet, err)
			continue
		}

		if seenTags.IsNew(msg.Tag) {
			mc.sendAnnounceWithTag(msg.Tag)
		}
		peer := net.JoinHostPort(from.IP.String(), HTTP_PORT)
		peers.Add(peer)
	}
}
