package peers

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	MULTICAST_ADDRESS = `224.3.45.67:15679`
	MULTICAST_PORT    = `15679`
	MULTICAST_DELAY   = 10 * time.Minute
)

type peerMap struct {
	*sync.Mutex
	peers  map[string]time.Time
	expire time.Duration
}

func newPeerMap(timeout time.Duration) peerMap {
	p := peerMap{
		Mutex:  &sync.Mutex{},
		peers:  make(map[string]time.Time),
		expire: timeout,
	}
	return p
}

func (p peerMap) add(peer string) {
	p.Lock()
	if _, ok := p.peers[peer]; !ok {
		log.Println("registered", peer, len(p.peers)+1)
	}
	// always update to reflect last seen timestamp
	p.peers[peer] = time.Now().UTC()
	p.Unlock()
}

func (p peerMap) GetPeerList() []string {
	p.Lock()
	peers := make([]string, 0, len(p.peers))
	for peer, t := range p.peers {
		if time.Since(t) > p.expire {
			// remove expired
			delete(p.peers, peer)
			continue
		}
		peers = append(peers, peer)
	}
	p.Unlock()
	return peers
}

// Search for a package in the peer network.
func (p peerMap) Search(ctx context.Context, r *url.URL) (host string) {
	newUrl := *r
	newUrl.Scheme = "http"
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	eg, ctx := errgroup.WithContext(ctx)
	found := make(chan string, 1)
	for _, peer := range p.GetPeerList() {
		newUrl.Host = peer
		log.Println("requesting peer:", peer, newUrl.String())
		urlNext := newUrl.String()
		eg.Go(func() error {
			req, err := http.NewRequestWithContext(ctx, http.MethodHead, urlNext, nil)
			if err != nil {
				return nil
			}
			resp, err := http.DefaultClient.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				select {
				case found <- urlNext:
				default:
				}
				cancel()
			}
			return nil
		})
	}
	go func() {
		eg.Wait()
		close(found)
	}()
	return <-found
}
