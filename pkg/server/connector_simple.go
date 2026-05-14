package server

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/splattner/vdcgo/pkg/protocol"
	"github.com/splattner/vdcgo/pkg/runtime"
)

func (c *Connector) handleSimpleLine(line string) error {
	m, err := protocol.ParseSimpleLine(line)
	if err != nil {
		return err
	}
	switch {
	case strings.HasPrefix(m.Cmd, "L"):
		log.Printf("device log %s", m.Value)
		return nil
	case strings.EqualFold(m.Cmd, "BYE"):
		d, err := c.registry.ResolveTag(m.Tag)
		if err != nil {
			return withStatusTag(m.Tag, err)
		}
		c.registry.Remove(d.Tag)
		return nil
	default:
		_, err := c.registry.ResolveTag(m.Tag)
		if err != nil {
			return withStatusTag(m.Tag, err)
		}
		return withStatusTag(m.Tag, c.handleDeviceSimple(m))
	}
}

func (c *Connector) handleDeviceSimple(m protocol.SimpleMessage) error {
	cmd := strings.ToUpper(m.Cmd)
	device, err := c.registry.ResolveTag(m.Tag)
	if err != nil {
		return err
	}
	tag := device.Tag

	if strings.HasPrefix(cmd, "C") {
		idx, err := parseSimpleIndexedCommand(cmd, 'C')
		if err != nil {
			return err
		}
		val, err := parseSimpleValue(m.Value)
		if err != nil {
			return err
		}
		c.registry.SetChannel(tag, idx, val)
		c.emitEvent(runtime.Event{Type: runtime.EventChannel, Tag: tag, UniqueID: device.UniqueID, Index: idx, Value: val})
		return nil
	}
	if strings.HasPrefix(cmd, "B") {
		idx, err := parseSimpleIndexedCommand(cmd, 'B')
		if err != nil {
			return err
		}
		val, err := parseSimpleValue(m.Value)
		if err != nil {
			return err
		}
		c.registry.SetButton(tag, idx, val)
		c.emitEvent(runtime.Event{Type: runtime.EventButton, Tag: tag, UniqueID: device.UniqueID, Index: idx, Value: val})
		c.emitButtonAction(tag, device.UniqueID, idx, val, "")
		return nil
	}
	if strings.HasPrefix(cmd, "I") {
		idx, err := parseSimpleIndexedCommand(cmd, 'I')
		if err != nil {
			return err
		}
		val, err := parseSimpleValue(m.Value)
		if err != nil {
			return err
		}
		c.registry.SetInput(tag, idx, val)
		c.emitEvent(runtime.Event{Type: runtime.EventInput, Tag: tag, UniqueID: device.UniqueID, Index: idx, Value: val})
		return nil
	}
	if strings.HasPrefix(cmd, "S") {
		idx, err := parseSimpleIndexedCommand(cmd, 'S')
		if err != nil {
			return err
		}
		val, err := parseSimpleValue(m.Value)
		if err != nil {
			return err
		}
		c.registry.SetSensor(tag, idx, val)
		c.emitEvent(runtime.Event{Type: runtime.EventSensor, Tag: tag, UniqueID: device.UniqueID, Index: idx, Value: val})
		return nil
	}
	if cmd == "SYNC" {
		c.registry.SetSync(tag, true)
		return nil
	}
	if cmd == "SYNCED" {
		c.registry.SetSync(tag, false)
		return nil
	}
	if cmd == "ACTIVE" {
		val, err := parseSimpleValue(m.Value)
		if err != nil {
			return err
		}
		c.registry.SetActive(tag, val != 0)
		c.emitEvent(runtime.Event{Type: runtime.EventActive, Tag: tag, UniqueID: device.UniqueID, Active: val != 0})
		return nil
	}
	if strings.HasPrefix(cmd, "P") {
		return nil
	}
	return fmt.Errorf("unknown message %q", m.Cmd)
}

func parseSimpleIndexedCommand(cmd string, prefix byte) (int, error) {
	if len(cmd) < 2 || cmd[0] != prefix {
		return 0, fmt.Errorf("invalid command %q", cmd)
	}
	idx, err := strconv.Atoi(cmd[1:])
	if err != nil {
		return 0, fmt.Errorf("invalid index in command %q", cmd)
	}
	return idx, nil
}

func parseSimpleValue(raw string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid numeric value %q", raw)
	}
	return v, nil
}
