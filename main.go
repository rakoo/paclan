//  Written in 2014 by Matthieu Rakotojaona <matthieu.rakotojaona {on}
//  gmail.com>
//
//  To the extent possible under law, the author(s) have dedicated all
//  copyright and related and neighboring rights to this software to the
//  public domain worldwide. This software is distributed without any
//  warranty.
//
//  You should have received a copy of the CC0 Public Domain Dedication
//  along with this software. If not, see
//  <http://creativecommons.org/publicdomain/zero/1.0/>.

package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/Kunde21/paclan/peers"
	"github.com/knq/ini"
)

const (
	TTL       = 1 * time.Hour
	HTTP_PORT = `15678`
	// Note that we only provide packages, not dbs
	PKG_CACHE_DIR = `/var/cache/pacman/pkg`
)

// peers = newPeerMap(10 * time.Minute)
var arch = ""

type server struct {
	*peers.DNS
	arch string
}

func main() {
	iface := flag.String("i", "", "network interface to serve on (i.e. eth0)")

	conf, err := ini.LoadFile("/etc/pacman.conf")
	if err != nil {
		log.Fatal(err)
	}
	arch := conf.GetKey("options.Architecture")
	if arch == "" || arch == "auto" {
		out, err := exec.Command("uname", "-m").CombinedOutput()
		if err != nil {
			log.Fatal(err)
		}
		arch = string(bytes.TrimSpace(out))
	}
	fmt.Println("arch:", arch)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := server{DNS: peers.NewDNS(*iface, HTTP_PORT, TTL), arch: arch}
	go func() {
		for {
			if err := srv.RegisterSelf(ctx, TTL); err != nil {
				log.Println("multicast error:", err)
			}
			if ctx.Err() != nil {
				log.Println("registration exiting")
				return
			}
		}
	}()
	go func() {
		for {
			if err := srv.Listen(ctx); err != nil {
				log.Println("listen error:", err)
			}
			if ctx.Err() != nil {
				log.Println("listener exiting")
				return
			}
		}
	}()

	srvHTTP := srv.serveHttp()

	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGTERM, os.Kill)
	select {
	case <-c:
		srv.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		log.Println("http server closing")
		srvHTTP.Shutdown(ctx)
	}
	log.Println("exiting...")
}

func (srv server) serveHttp() *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/", http.HandlerFunc(srv.handle))
	srvHTTP := &http.Server{
		Addr:              net.JoinHostPort("", HTTP_PORT),
		Handler:           mux,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	go func() {
		log.Println("Serving from", HTTP_PORT)
		err := srvHTTP.ListenAndServe()
		if err != nil {
			log.Fatal(err)
		}
	}()
	return srvHTTP
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
	log.Println("remote request:", path.Base(path.Dir(r.URL.Path)))
	dir, file := path.Split(r.URL.Path)
	fpath := path.Join(PKG_CACHE_DIR, file)
	if srv.arch != path.Base(dir) {
		log.Println("pkg search:", path.Base(dir), file, "arch mismatch")
		w.WriteHeader(http.StatusNotFound)
		return
	}
	_, err := os.Stat(fpath)
	if err != nil {
		log.Println("pkg search:", file, err)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	log.Println("pkg found:", file, err)
	switch r.Method {
	case http.MethodHead:
		w.WriteHeader(http.StatusOK)
	case http.MethodGet:
		http.ServeFile(w, r, fpath)
	}
}
