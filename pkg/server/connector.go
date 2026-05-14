package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"

	"github.com/splattner/vdcgo/pkg/protocol"
	"github.com/splattner/vdcgo/pkg/runtime"
)

type protocolMode int

const (
	modeJSON protocolMode = iota
	modeSimple
)

type Connector struct {
	conn         net.Conn
	registry     *runtime.Registry
	vdcInfo      protocol.InitVDCMessage
	mode         protocolMode
	onEvent      func(runtime.Event)
	writeMu      sync.Mutex
	buttonStates map[string]buttonSequenceState
}

func NewConnector(conn net.Conn, onEvent func(runtime.Event)) *Connector {
	return &Connector{
		conn:         conn,
		mode:         modeJSON,
		registry:     runtime.NewRegistry(),
		onEvent:      onEvent,
		buttonStates: make(map[string]buttonSequenceState),
	}
}

func (c *Connector) emitEvent(e runtime.Event) {
	if c.onEvent == nil {
		return
	}
	e.Connection = c.conn.RemoteAddr().String()
	c.onEvent(e)
}

func (c *Connector) writeLine(line string) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err := io.WriteString(c.conn, line+"\n")
	return err
}

func (c *Connector) SendLightLevel(uniqueID string, value float64) (bool, error) {
	return c.SendChannelValue(uniqueID, 0, value)
}

// SendChannelValue sends a channel value to the device addressed by uniqueID.
// Returns (true, nil) if the device was found and the message dispatched.
// The state store is updated optimistically so the UI reflects the commanded
// value immediately, even if the external device has not echoed it back yet.
func (c *Connector) SendChannelValue(uniqueID string, channelIndex int, value float64) (bool, error) {
	dev, ok := c.registry.FindByUniqueID(uniqueID)
	if !ok {
		return false, nil
	}
	if c.mode == modeSimple {
		if err := c.writeLine(fmt.Sprintf("%s:C%d=%f", dev.Tag, channelIndex, value)); err != nil {
			return false, err
		}
		c.emitEvent(runtime.Event{Type: runtime.EventChannel, UniqueID: uniqueID, Index: channelIndex, Value: value})
		return true, nil
	}
	msg := map[string]any{
		"message": "channel",
		"index":   channelIndex,
		"value":   value,
	}
	if dev.Tag != "" {
		msg["tag"] = dev.Tag
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return false, err
	}
	if err := c.writeLine(string(b)); err != nil {
		return false, err
	}
	c.emitEvent(runtime.Event{Type: runtime.EventChannel, UniqueID: uniqueID, Index: channelIndex, Value: value})
	return true, nil
}

func (c *Connector) Run(ctx context.Context) {
	go func() {
		<-ctx.Done()
		_ = c.conn.Close()
	}()

	scanner := bufio.NewScanner(c.conn)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if err := c.handleLine(line); err != nil {
			tag, cause := splitStatusError(err)
			if serr := c.sendStatus(cause, tag); serr != nil {
				log.Printf("connector status write failed: %v", serr)
			}
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) && !isExpectedConnShutdownErr(err) {
		log.Printf("connector scanner error: %v", err)
	}

	// Connection closed — vanish any devices that did not send an explicit "bye".
	for _, dev := range c.registry.AllDevices() {
		c.emitEvent(runtime.Event{Type: runtime.EventRemove, Tag: dev.Tag, UniqueID: dev.UniqueID})
	}
}

func (c *Connector) handleLine(line string) error {
	if c.mode == modeSimple {
		return c.handleSimpleLine(line)
	}
	return c.handleJSONLine(line)
}
