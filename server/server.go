package server

import (
	"context"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"runtime"
	"sync"
	"time"

	"github.com/Kunde21/paclan/config"
	"golang.org/x/sync/errgroup"
)

const (
	ARCH_HEADER        = `X-Arch-Req`
	PACMAN_CONFIG_FILE = `/etc/pacman.conf`
)

type server struct {
	*http.Server
	// *peers.DNS
	PeerLister
	arch  string
	cache string
}

type PeerLister interface {
	GetPeerList() []string
}

func New(conf *config.Paclan, peers PeerLister) (server, error) {
	srv := server{
		PeerLister: peers,
		arch:       conf.Arch,
		cache:      conf.CacheDir,
	}
	mux := http.NewServeMux()
	mux.Handle("/", http.HandlerFunc(srv.handle))
	srv.Server = &http.Server{
		Addr:              net.JoinHostPort("", conf.HTTPPort),
		Handler:           mux,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	return srv, nil
}

func (srv server) handle(w http.ResponseWriter, r *http.Request) {
	addr, err := net.ResolveTCPAddr("tcp", r.RemoteAddr)
	if err != nil {
		log.Printf("Error serving %s: %s\n", r.RemoteAddr, err)
		return
	}
	if addr.IP.IsLoopback() {
		srv.handleLocal(w, r)
	} else {
		srv.handleRemote(w, r)
	}
}

func (srv server) handleLocal(w http.ResponseWriter, r *http.Request) {
	log.Println("local request:", r.URL.Path)
	found := make(chan string, 1)
	defer close(found)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	redir := srv.Search(ctx, r.URL)
	if redir == "" {
		log.Println("not found", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	log.Println("found", r.URL, redir)
	switch r.Method {
	case http.MethodHead:
		w.WriteHeader(http.StatusOK)
	case http.MethodGet:
		http.Redirect(w, r, redir, http.StatusFound)
		return
	}
}

func (srv server) searchRemote(ctx context.Context, r *url.URL, found chan<- string) {
	path := path.Join(path.Dir(path.Dir(r.Path)), runtime.GOARCH, path.Base(r.Path))
	newUrl := *r
	newUrl.Scheme = "http"
	newUrl.Path = path
	p := srv.GetPeerList()
	wg := &sync.WaitGroup{}
	wg.Add(len(p))
	for _, peer := range p {
		newUrl.Host = peer
		log.Println("requesting peer:", p, newUrl.String())
		go func(url string) {
			defer wg.Done()
			req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
			if err != nil {
				return
			}
			resp, err := http.DefaultClient.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				found <- url
			}
		}(newUrl.String())
	}
	wg.Wait()
}

func (srv server) handleRemote(w http.ResponseWriter, r *http.Request) {
	log.Println("remote request:", path.Base(r.URL.Path))
	file := path.Base(r.URL.Path)
	fpath := path.Join(srv.cache, file)
	if arch := r.Header.Get(ARCH_HEADER); arch != "" && srv.arch != arch {
		log.Println("pkg search:", arch, file, "arch mismatch")
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if _, err := os.Stat(fpath); err != nil {
		log.Println("pkg search:", file, err)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	log.Println("pkg found:", file)
	switch r.Method {
	case http.MethodHead:
		w.WriteHeader(http.StatusOK)
	case http.MethodGet:
		http.ServeFile(w, r, fpath)
	}
}

// Search for a package in the peer network.
func (srv server) Search(ctx context.Context, r *url.URL) (host string) {
	newUrl := *r
	newUrl.Scheme = "http"
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	eg, ctx := errgroup.WithContext(ctx)
	found := make(chan string, 1)
	peers := srv.GetPeerList()
	log.Printf("requesting pkg %q from %d peers", path.Base(r.Path), len(peers))
	for _, peer := range peers {
		newUrl.Host = peer
		urlNext := newUrl.String()
		eg.Go(func() error {
			req, err := http.NewRequestWithContext(ctx, http.MethodHead, urlNext, nil)
			if err != nil {
				return nil
			}
			req.Header.Add(ARCH_HEADER, srv.arch)
			resp, err := http.DefaultClient.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				select {
				case found <- urlNext:
					cancel() // use first found instance
				default:
				}
			}
			return nil
		})
	}
	go func() { // peform cleanup in the background
		eg.Wait()
		close(found)
	}()
	return <-found
}
