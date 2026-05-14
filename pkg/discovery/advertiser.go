package discovery

import (
	"fmt"

	"github.com/grandcat/zeroconf"
)

const ServiceTypeDSVDC = "_ds-vdc._tcp"

// Config defines DNS-SD advertisement details.
type Config struct {
	Instance string
	Port     int
	DSUID    string
	NoAuto   bool
}

// Advertiser publishes DNS-SD service records.
type Advertiser struct {
	server *zeroconf.Server
}

func Start(cfg Config) (*Advertiser, error) {
	if cfg.Instance == "" {
		cfg.Instance = "vdcgo"
	}
	if cfg.Port <= 0 {
		return nil, fmt.Errorf("invalid discovery port %d", cfg.Port)
	}
	if cfg.DSUID == "" {
		return nil, fmt.Errorf("missing dSUID for discovery")
	}

	txt := []string{"dSUID=" + cfg.DSUID}
	if cfg.NoAuto {
		// vdcd publishes noauto as empty-value TXT record.
		txt = append(txt, "noauto=")
	}

	srv, err := zeroconf.Register(cfg.Instance, ServiceTypeDSVDC, "local.", cfg.Port, txt, nil)
	if err != nil {
		return nil, fmt.Errorf("register dns-sd service: %w", err)
	}
	return &Advertiser{server: srv}, nil
}

func (a *Advertiser) Shutdown() {
	if a == nil || a.server == nil {
		return
	}
	a.server.Shutdown()
}
