package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/splattner/vdcgo/pkg/runtime"
)

// Config contains listener configuration.
type Config struct {
	Listen   string
	NonLocal bool
	OnEvent  func(runtime.Event)
}

// Server hosts external device API connectors.
type Server struct {
	cfg        Config
	mu         sync.RWMutex
	connectors map[*Connector]struct{}
}

func New(cfg Config) (*Server, error) {
	if strings.TrimSpace(cfg.Listen) == "" {
		cfg.Listen = "8999"
	}
	return &Server{cfg: cfg, connectors: make(map[*Connector]struct{})}, nil
}

// SetLightLevel sends a brightness channel command to the external device with the given uniqueid.
func (s *Server) SetLightLevel(uniqueID string, value float64) error {
	return s.SetChannelValue(uniqueID, 0, value)
}

// SetChannelValue sends a channel value command for the given channel index
// to the external device with the given uniqueid.
func (s *Server) SetChannelValue(uniqueID string, channelIndex int, value float64) error {
	s.mu.RLock()
	conns := make([]*Connector, 0, len(s.connectors))
	for c := range s.connectors {
		conns = append(conns, c)
	}
	s.mu.RUnlock()

	for _, c := range conns {
		if ok, err := c.SendChannelValue(uniqueID, channelIndex, value); err != nil {
			return err
		} else if ok {
			return nil
		}
	}
	return fmt.Errorf("device with uniqueid %q not connected", uniqueID)
}

func (s *Server) Run(ctx context.Context) error {
	network, address, cleanup, err := s.listenSpec()
	if err != nil {
		return err
	}
	defer cleanup()

	ln, err := net.Listen(network, address)
	if err != nil {
		return fmt.Errorf("listen failed: %w", err)
	}
	defer ln.Close()

	var wg sync.WaitGroup
	acceptErr := make(chan error, 1)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				acceptErr <- err
				return
			}
			if !s.cfg.NonLocal {
				if tcpAddr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
					if !tcpAddr.IP.IsLoopback() {
						_ = conn.Close()
						log.Printf("rejected non-local connection from %s", tcpAddr.IP)
						continue
					}
				}
			}
			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				connector := NewConnector(c, s.cfg.OnEvent)
				s.mu.Lock()
				s.connectors[connector] = struct{}{}
				s.mu.Unlock()
				defer func() {
					s.mu.Lock()
					delete(s.connectors, connector)
					s.mu.Unlock()
				}()
				connector.Run(ctx)
			}(conn)
		}
	}()

	select {
	case <-ctx.Done():
		_ = ln.Close()
		wg.Wait()
		return nil
	case err := <-acceptErr:
		_ = ln.Close()
		wg.Wait()
		return fmt.Errorf("accept failed: %w", err)
	}
}

func (s *Server) listenSpec() (network string, address string, cleanup func(), err error) {
	cleanup = func() {}
	listen := strings.TrimSpace(s.cfg.Listen)
	if strings.HasPrefix(listen, "/") {
		dir := filepath.Dir(listen)
		if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
			return "", "", cleanup, fmt.Errorf("cannot create socket directory: %w", mkErr)
		}
		if rmErr := os.Remove(listen); rmErr != nil && !os.IsNotExist(rmErr) {
			return "", "", cleanup, fmt.Errorf("cannot clear existing socket path: %w", rmErr)
		}
		cleanup = func() { _ = os.Remove(listen) }
		return "unix", listen, cleanup, nil
	}
	if strings.Contains(listen, ":") {
		return "tcp", listen, cleanup, nil
	}
	return "tcp", ":" + listen, cleanup, nil
}
