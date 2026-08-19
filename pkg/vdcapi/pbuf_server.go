package vdcapi

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"

	"github.com/splattner/vdcgo/pkg/logging"
	"github.com/splattner/vdcgo/pkg/runtime"
)

const (
	pbufMaxMessageSize = 16384

	pbufTypeGenericResponse                 = 1
	pbufTypeVdsmRequestHello                = 2
	pbufTypeVdcResponseHello                = 3
	pbufTypeVdsmRequestGetProp              = 4
	pbufTypeVdcResponseGetProp              = 5
	pbufTypeVdsmRequestSetProp              = 6
	pbufTypeVdsmSendPing                    = 8
	pbufTypeVdcSendPong                     = 9
	pbufTypeVdcSendAnnounceDevice           = 10
	pbufTypeVdcSendVanish                   = 11
	pbufTypeVdcSendPushNotification         = 12
	pbufTypeVdsmSendRemove                  = 13
	pbufTypeVdsmSendBye                     = 14
	pbufTypeVdsmNotifyCallScene             = 15
	pbufTypeVdsmNotifySaveScene             = 16
	pbufTypeVdsmNotifyUndoScene             = 17
	pbufTypeVdsmNotifySetLocalPrio          = 18
	pbufTypeVdsmNotifyCallMinScene          = 19
	pbufTypeVdsmNotifyIdentify              = 20
	pbufTypeVdsmNotifySetControlValue       = 21
	pbufTypeVdcSendIdentify                 = 22
	pbufTypeVdcSendAnnounceVdc              = 23
	pbufTypeVdsmNotifyDimChannel            = 24
	pbufTypeVdsmNotifySetOutputChannelValue = 25
	pbufTypeVdsmRequestGenericReq           = 26

	pbufResultOK                  = 0
	pbufResultMessageUnknown      = 1
	pbufResultIncompatibleAPI     = 2
	pbufResultServiceUnavailable  = 3
	pbufResultInsufficientStorage = 4
	pbufResultForbidden           = 5
	pbufResultNotImplemented      = 6
	pbufResultNoContentForArray   = 7
	pbufResultInvalidValueType    = 8
	pbufResultMissingSubmessage   = 9
	pbufResultMissingData         = 10
	pbufResultNotFound            = 11
	pbufResultNotAuthorized       = 12
)

// PbufServer implements a minimal protobuf vDC API endpoint.
type PbufServer struct {
	ServerConfig

	sessionMu     sync.Mutex
	sessionConn   net.Conn
	sessionDSUID  string
	statusSession statusSession
}

type pbufPropertyElement struct {
	Name     string
	Value    any
	Elements []pbufPropertyElement
}

func (s *PbufServer) Run(ctx context.Context) error {
	if s.Port <= 0 {
		return fmt.Errorf("invalid vdc api port %d", s.Port)
	}
	if s.DSUID == "" {
		return fmt.Errorf("missing dSUID")
	}

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", s.Port))
	if err != nil {
		return fmt.Errorf("listen on protobuf vdc api port %d: %w", s.Port, err)
	}
	defer ln.Close()
	if s.RampManager != nil {
		defer s.RampManager.stopAll()
	}

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
			enableTCPKeepalive(conn)
			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				s.handleConn(ctx, c)
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
		return fmt.Errorf("protobuf vdc api accept failed: %w", err)
	}
}

func (s *PbufServer) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	defer s.releaseSessionOwner(conn)
	logging.Info("pbuf_client_connected", logging.Fields{"remote_addr": conn.RemoteAddr().String()})
	defer func() {
		s.statusSession.setDisconnected()
		logging.Info("pbuf_client_disconnected", logging.Fields{"remote_addr": conn.RemoteAddr().String()})
	}()

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	defer close(done)

	sess := session{}
	var sessMu sync.RWMutex
	var writeMu sync.Mutex
	writeFrame := func(frame []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_, err := conn.Write(frame)
		if err == nil {
			s.traceTxFrames([][]byte{frame})
		}
		return err
	}

	if s.State != nil {
		subID, updates := s.State.Subscribe()
		defer s.State.Unsubscribe(subID)
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-done:
					return
				case up, ok := <-updates:
					if !ok {
						return
					}
					sessMu.RLock()
					active := sess.active
					sessMu.RUnlock()
					if !active {
						continue
					}
					dsuid := deviceDSUID(s.DSUID, up.Device, up.Device.Key)
					switch up.Type {
					case runtime.EventInit:
						logging.Info("pbuf_announce_device", logging.Fields{
							"dsuid":     dsuid,
							"unique_id": up.Device.UniqueID,
							"name":      up.Device.Name,
							"output":    up.Device.Output,
							"active":    up.Device.Active,
						})
						// Vanish first so vdcd/dSS discards any cached configuration
						// for this device and performs a full getProperty re-query on
						// the subsequent AnnounceDevice. This is important when
						// EventInit fires a second time after descriptor metadata has
						// been pushed (e.g. sensor types after plugin activation).
						if err := writeFrame(buildPbufVanish(dsuid)); err != nil {
							_ = conn.Close()
							return
						}
						if err := writeFrame(buildPbufAnnounceDevice(dsuid, s.DSUID)); err != nil {
							_ = conn.Close()
							return
						}
						changed := changedStatePayload(up.Device)
						logging.Debug("pbuf_push_notification_sent", logging.Fields{
							"dsuid":  dsuid,
							"active": up.Device.Active,
							"event":  "init",
						})
						if err := writeFrame(buildPbufPushNotification(dsuid, changed)); err != nil {
							_ = conn.Close()
							return
						}
						if s.Config != nil {
							s.Config.MarkDSUIDAdded(dsuid)
						}

					case runtime.EventRemove:
						logging.Info("pbuf_vanish_device", logging.Fields{"dsuid": dsuid, "unique_id": up.Device.UniqueID})
						if err := writeFrame(buildPbufVanish(dsuid)); err != nil {
							_ = conn.Close()
							return
						}
						if s.Config != nil {
							s.Config.MarkDSUIDRemoved(dsuid)
						}

					case runtime.EventActive:
						logging.Info("pbuf_push_active_state", logging.Fields{"dsuid": dsuid, "active": up.Device.Active})
						changed := changedStatePayload(up.Device)
						if err := writeFrame(buildPbufPushNotification(dsuid, changed)); err != nil {
							_ = conn.Close()
							return
						}
					case runtime.EventChannel, runtime.EventButton, runtime.EventButtonAction, runtime.EventInput, runtime.EventSensor:
						logging.Debug("pbuf_push_notification_sent", logging.Fields{
							"dsuid":  dsuid,
							"active": up.Device.Active,
							"event":  up.Type,
						})
						changed := changedStatePayload(up.Device)
						if err := writeFrame(buildPbufPushNotification(dsuid, changed)); err != nil {
							_ = conn.Close()
							return
						}
					}
				}
			}
		}()
	}

	for {
		payload, err := readPbufFrame(conn)
		if err != nil {
			if errors.Is(err, io.EOF) || isExpectedConnShutdownErr(err) {
				logging.Debug("pbuf_client_disconnected", logging.Fields{"remote_addr": conn.RemoteAddr().String()})
				return
			}
			logging.Warn("pbuf_read_error", logging.Fields{"remote_addr": conn.RemoteAddr().String(), "error": err})
			return
		}
		sessMu.Lock()
		frames, closeAfter := s.processPbufMessageForConn(payload, &sess, conn)
		sessMu.Unlock()
		for _, f := range frames {
			if err := writeFrame(f); err != nil {
				if !isExpectedConnShutdownErr(err) {
					logging.Warn("pbuf_write_error", logging.Fields{"remote_addr": conn.RemoteAddr().String(), "error": err})
				}
				return
			}
		}
		if closeAfter {
			return
		}
	}
}

func (s *PbufServer) claimSessionOwner(conn net.Conn, vdsmDSUID string) (activeDSUID string, replacedConn net.Conn, ok bool) {
	if conn == nil {
		return "", nil, true
	}
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()

	if s.sessionConn == nil {
		s.sessionConn = conn
		s.sessionDSUID = vdsmDSUID
		return "", nil, true
	}
	if s.sessionConn == conn {
		s.sessionDSUID = vdsmDSUID
		return "", nil, true
	}
	if strings.EqualFold(s.sessionDSUID, vdsmDSUID) {
		replacedConn = s.sessionConn
		s.sessionConn = conn
		s.sessionDSUID = vdsmDSUID
		return "", replacedConn, true
	}
	return s.sessionDSUID, nil, false
}

func (s *PbufServer) isSessionOwner(conn net.Conn) bool {
	if conn == nil {
		return true
	}
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	return s.sessionConn == conn
}

func (s *PbufServer) releaseSessionOwner(conn net.Conn) {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	if s.sessionConn == conn {
		s.sessionConn = nil
		s.sessionDSUID = ""
	}
}

func readPbufFrame(r io.Reader) ([]byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}
	n := int(binary.BigEndian.Uint16(header))
	if n <= 0 || n > pbufMaxMessageSize {
		return nil, fmt.Errorf("invalid protobuf message size %d", n)
	}
	p := make([]byte, n)
	if _, err := io.ReadFull(r, p); err != nil {
		return nil, err
	}
	return p, nil
}
