package vdcapi

import (
	"net"
	"sync"
	"time"
)

// statusSession holds the current DSS connection state for the status file.
type statusSession struct {
	mu          sync.RWMutex
	connected   bool
	remoteAddr  string
	vdsmDSUID   string
	apiVersion  int
	connectedAt time.Time
}

func (ss *statusSession) setConnected(conn net.Conn, vdsmDSUID string, apiVersion int) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.connected = true
	if conn != nil {
		ss.remoteAddr = conn.RemoteAddr().String()
	}
	ss.vdsmDSUID = vdsmDSUID
	ss.apiVersion = apiVersion
	ss.connectedAt = time.Now()
}

func (ss *statusSession) setDisconnected() {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.connected = false
}

func (ss *statusSession) snapshot() SessionSnapshot {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return SessionSnapshot{
		Connected:   ss.connected,
		RemoteAddr:  ss.remoteAddr,
		VdsmDSUID:   ss.vdsmDSUID,
		APIVersion:  ss.apiVersion,
		ConnectedAt: ss.connectedAt,
	}
}

// SessionSnapshot is the JSON-serialisable vDSM session state exposed via /api/dss.
type SessionSnapshot struct {
	Connected   bool      `json:"connected"`
	RemoteAddr  string    `json:"remote_addr,omitempty"`
	VdsmDSUID   string    `json:"vdsm_dsuid,omitempty"`
	APIVersion  int       `json:"api_version,omitempty"`
	ConnectedAt time.Time `json:"connected_at,omitempty"`
}

// Session returns a snapshot of the current vDSM session state.
func (s *PbufServer) Session() SessionSnapshot {
	return s.statusSession.snapshot()
}
