package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/splattner/vdcgo/pkg/logging"
	"github.com/splattner/vdcgo/pkg/vdcgo"
)

func main() {
	nonLocal := flag.Bool("non-local", false, "allow non-local HTTP API clients")
	vdcAPI := flag.Int("vdcapi-port", 0, "vDC API listen port (default: 8340)")
	noVdcAPI := flag.Bool("novdcapi", false, "disable vDC API stub listener")
	noDiscovery := flag.Bool("nodiscovery", false, "disable DNS-SD discovery advertisement")
	avahiDBus := flag.Bool("avahi-dbus", false, "advertise via Avahi D-Bus instead of direct multicast (use inside containers with host D-Bus mounted)")
	noAuto := flag.Bool("noauto", false, "publish vdc as noauto")
	dsuid := flag.String("dsuid", "", "34-hex-digit dSUID advertised via DNS-SD")
	description := flag.String("description", "vdcgo external", "DNS-SD instance description")
	vendor := flag.String("vendor", "github.com/splattner", "vDC vendor identifier")
	model := flag.String("model", "vdcgo", "vDC model identifier")
	dataDir := flag.String("datadir", "", "directory for persistent data (scenes, device config); empty disables persistence")
	httpListen := flag.String("http-listen", envOrDefault("VDCGO_HTTP_LISTEN", ""), "address for the REST/WebSocket HTTP API, e.g. :8090 (empty = disabled)")
	flag.Parse()

	svc, err := vdcgo.NewService(vdcgo.Config{
		NonLocal:     *nonLocal,
		VdcAPIPort:   *vdcAPI,
		EnableVdcAPI: !*noVdcAPI,
		EnableDNSSD:  !*noDiscovery,
		UseAvahiDBus: *avahiDBus,
		DSUID:        *dsuid,
		Description:  *description,
		Vendor:       *vendor,
		Model:        *model,
		NoAuto:       *noAuto,
		DataDir:      *dataDir,
		HTTPListen:   *httpListen,
	})
	if err != nil {
		logging.Error("daemon_config_error", logging.Fields{"error": err})
		log.Fatalf("config error: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logging.Info("daemon_start", logging.Fields{
		"vdcapi_port":   svc.Config().VdcAPIPort,
		"dns_discovery": !*noDiscovery,
		"dsuid":         svc.Config().DSUID,
		"http_listen":   *httpListen,
	})
	if err := svc.Run(ctx); err != nil {
		logging.Error("daemon_stopped_with_error", logging.Fields{"error": err})
		log.Fatalf("daemon stopped with error: %v", err)
	}
	logging.Info("daemon_stopped", nil)
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
