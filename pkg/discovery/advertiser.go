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
	// UseAvahiDBus instructs the advertiser to publish via the Avahi D-Bus API
	// instead of opening raw multicast sockets directly. Use this when running
	// inside a container (e.g. Home Assistant add-on) where multicast does not
	// reach the host network but /var/run/dbus is bind-mounted from the host.
	UseAvahiDBus bool
}

// Advertiser publishes DNS-SD service records. Exactly one of the two backend
// fields is non-nil, depending on which backend was selected at Start time.
type Advertiser struct {
	zeroconf *zeroconf.Server // direct multicast (default)
	avahi    *avahiHandle     // Avahi D-Bus proxy
}

// Start registers the DNS-SD service using whichever backend is configured.
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

	if cfg.UseAvahiDBus {
		h, err := startViaAvahi(cfg)
		if err != nil {
			return nil, err
		}
		return &Advertiser{avahi: h}, nil
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
	return &Advertiser{zeroconf: srv}, nil
}

// Shutdown deregisters the DNS-SD service and releases all resources.
func (a *Advertiser) Shutdown() {
	if a == nil {
		return
	}
	if a.zeroconf != nil {
		a.zeroconf.Shutdown()
	}
	a.avahi.shutdown()
}
