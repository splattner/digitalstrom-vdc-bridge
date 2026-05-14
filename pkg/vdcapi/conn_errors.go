package vdcapi

import (
	"errors"
	"net"
	"strings"
	"time"
)

// enableTCPKeepalive sets SO_KEEPALIVE on the connection so that a firewall
// or NAT device dropping idle flows is detected within ~30 s instead of the
// OS default (often hours).
func enableTCPKeepalive(conn net.Conn) {
	tc, ok := conn.(*net.TCPConn)
	if !ok {
		return
	}
	_ = tc.SetKeepAlive(true)
	_ = tc.SetKeepAlivePeriod(30 * time.Second)
}

func isExpectedConnShutdownErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "closed network connection") || strings.Contains(msg, "read/write on closed pipe")
}
