package discovery

import (
	"errors"
	"fmt"

	"github.com/godbus/dbus/v5"
)

const (
	avahiBusName     = "org.freedesktop.Avahi"
	avahiServerPath  = dbus.ObjectPath("/")
	avahiServerIface = "org.freedesktop.Avahi.Server"
	avahiEGIface     = "org.freedesktop.Avahi.EntryGroup"

	// AVAHI_IF_UNSPEC / AVAHI_PROTO_UNSPEC — let Avahi choose interface and protocol.
	avahiIfUnspec    = int32(-1)
	avahiProtoUnspec = int32(-1)
)

// ErrAvahiUnavailable is returned when the Avahi daemon is not running or its
// D-Bus service name has no owner. The caller may fall back to direct multicast.
var ErrAvahiUnavailable = errors.New("avahi daemon not available on D-Bus")

// avahiHandle holds the D-Bus resources for an active Avahi advertisement.
type avahiHandle struct {
	conn  *dbus.Conn
	group dbus.BusObject
}

// startViaAvahi registers the DNS-SD service through the host Avahi daemon
// over D-Bus. This is the correct approach when running inside a Docker
// container (e.g. as a Home Assistant add-on) where multicast does not reach
// the host network: D-Bus is mounted from the host so Avahi can publish the
// record on the real network interface.
func startViaAvahi(cfg Config) (*avahiHandle, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, fmt.Errorf("connect to system dbus: %w", err)
	}

	// Check whether avahi-daemon is actually running before going further.
	// An absent name owner means Avahi is not running; return ErrAvahiUnavailable
	// so the caller can fall back gracefully instead of crashing.
	var nameOwner string
	if err := conn.BusObject().Call("org.freedesktop.DBus.GetNameOwner", 0, avahiBusName).Store(&nameOwner); err != nil {
		_ = conn.Close()
		return nil, ErrAvahiUnavailable
	}

	// Ask Avahi to create a new empty entry group.
	avahiObj := conn.Object(avahiBusName, avahiServerPath)
	var groupPath dbus.ObjectPath
	if err := avahiObj.Call(avahiServerIface+".EntryGroupNew", 0).Store(&groupPath); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("avahi EntryGroupNew: %w", err)
	}

	group := conn.Object(avahiBusName, groupPath)

	// Build TXT record bytes: each entry is a "key=value" byte slice.
	txt := [][]byte{[]byte("dSUID=" + cfg.DSUID)}
	if cfg.NoAuto {
		txt = append(txt, []byte("noauto="))
	}

	// AddService(iface, proto, flags, name, type, domain, host, port, txt)
	// Empty domain → ".local.", empty host → local hostname.
	err = group.Call(avahiEGIface+".AddService", 0,
		avahiIfUnspec,
		avahiProtoUnspec,
		uint32(0),
		cfg.Instance,
		ServiceTypeDSVDC,
		"", // domain  (default: local.)
		"", // host    (default: local hostname)
		uint16(cfg.Port),
		txt,
	).Err
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("avahi AddService: %w", err)
	}

	if err := group.Call(avahiEGIface+".Commit", 0).Err; err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("avahi Commit: %w", err)
	}

	return &avahiHandle{conn: conn, group: group}, nil
}

func (h *avahiHandle) shutdown() {
	if h == nil {
		return
	}
	_ = h.group.Call(avahiEGIface+".Free", 0).Err
	_ = h.conn.Close()
}
