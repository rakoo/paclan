package config

import (
	"bytes"
	"fmt"
	"os/exec"
	"time"

	"github.com/knq/ini"
)

const (
	TTL               = 1 * time.Hour
	HTTP_PORT         = `15678`
	MULTICAST_ADDRESS = `224.3.45.67`
	MULTICAST_PORT    = `15679`
	CACHE_DIR         = `/var/cache/pacman/pkg/`
	PACMAN_CONFIG     = `/etc/pacman.conf`
)

type Paclan struct {
	IFace         string
	HTTPPort      string
	Arch          string
	CacheDir      string
	MulticastPort string
	MulticastAddr string
	PeerTimeout   time.Duration
}

func New(confFile string, args []string) (*Paclan, error) {
	plConf, err := ini.LoadFile(confFile)
	if err != nil {
		return nil, err
	}
	conf := &Paclan{
		IFace:    plConf.GetKey("http.Interface"),
		HTTPPort: plConf.GetKey("http.Port"),
	}
	if conf.HTTPPort == "" {
		conf.HTTPPort = HTTP_PORT
	}
	conf.MulticastAddr = plConf.GetKey("multicast.Address")
	if conf.MulticastAddr == "" {
		conf.MulticastAddr = MULTICAST_ADDRESS
	}
	conf.MulticastPort = plConf.GetKey("multicast.Port")
	if conf.MulticastPort == "" {
		conf.MulticastPort = MULTICAST_PORT
	}
	if ttl := plConf.GetKey("multicast.TTL"); ttl != "" {
		ttlDur, err := time.ParseDuration(ttl)
		if err != nil {
			return nil, err
		}
		conf.PeerTimeout = ttlDur
	}
	if conf.PeerTimeout == 0 {
		conf.PeerTimeout = TTL
	}
	pacConf := plConf.GetKey("pacman.Config")
	if pacConf == "" {
		pacConf = PACMAN_CONFIG
	}
	return conf.pacmanConf(pacConf)
}

func (p Paclan) pacmanConf(file string) (*Paclan, error) {
	plConf, err := ini.LoadFile(file)
	if err != nil {
		return nil, err
	}
	p.CacheDir = plConf.GetKey("CacheDir")
	if p.CacheDir == "" {
		p.CacheDir = CACHE_DIR
	}
	p.Arch = plConf.GetKey("options.Architecture")
	if p.Arch == "" || p.Arch == "auto" {
		out, err := exec.Command("uname", "-m").CombinedOutput()
		if err != nil {
			return nil, err
		}
		p.Arch = string(bytes.TrimSpace(out))
	}
	fmt.Println("arch:", p.Arch)
	return &p, nil
}
