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
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Kunde21/paclan/config"
	"github.com/Kunde21/paclan/peers"
	"github.com/Kunde21/paclan/server"
)

func main() {
	const PaclanConfig = `/etc/pacman.d/paclan.conf`
	var confFile string
	flag.StringVar(&confFile, "c", PaclanConfig, "paclan configuration file")
	flag.Parse()
	conf, err := config.New(confFile, flag.Args())
	if err != nil {
		log.Fatal(err)
	}
	peers := peers.New(conf)
	srv, err := server.New(conf, peers)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer cancel()
		log.Println("Serving from", conf.HTTPPort)
		if err := srv.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()
	peers.Serve(ctx)

	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGTERM, os.Kill)
	select {
	case <-ctx.Done():
	case <-c:
	}
	peers.Close()
	shCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	log.Println("http server closing")
	srv.Shutdown(shCtx)
	log.Println("exiting...")
}
