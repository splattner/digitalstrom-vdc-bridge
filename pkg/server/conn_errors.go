package server

import (
	"errors"
	"net"
	"strings"
)

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
