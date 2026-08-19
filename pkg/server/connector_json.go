package server

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/splattner/vdcgo/pkg/protocol"
	"github.com/splattner/vdcgo/pkg/runtime"
)

func (c *Connector) handleJSONLine(line string) error {
	var raw any
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	var objects []map[string]any
	switch v := raw.(type) {
	case map[string]any:
		objects = append(objects, v)
	case []any:
		for i := range v {
			o, ok := v[i].(map[string]any)
			if !ok {
				return fmt.Errorf("JSON array element %d is not an object", i)
			}
			objects = append(objects, o)
		}
	default:
		return fmt.Errorf("message must be JSON object or array")
	}

	for _, obj := range objects {
		msg, _ := obj["message"].(string)
		if msg == "" {
			return fmt.Errorf("missing message field")
		}
		switch msg {
		case protocol.MessageInit:
			b, _ := json.Marshal(obj)
			var initMsg protocol.InitMessage
			if err := json.Unmarshal(b, &initMsg); err != nil {
				return withStatusTag(initMsg.Tag, fmt.Errorf("invalid init message: %w", err))
			}
			if err := protocol.ValidateInit(initMsg); err != nil {
				return withStatusTag(initMsg.Tag, err)
			}
			if c.registry.Count() > 0 && initMsg.Tag == "" {
				return withStatusTag(initMsg.Tag, fmt.Errorf("missing tag (needed for multiple devices on this connection)"))
			}
			if err := c.registry.Add(runtime.Device{
				Tag:      initMsg.Tag,
				UniqueID: initMsg.UniqueID,
				Output:   initMsg.Output,
				Name:     initMsg.Name,
			}); err != nil {
				return withStatusTag(initMsg.Tag, err)
			}
			c.emitEvent(runtime.Event{
				Type:     runtime.EventInit,
				Tag:      initMsg.Tag,
				UniqueID: initMsg.UniqueID,
				Name:     initMsg.Name,
				Output:   initMsg.Output,
			})
			if c.registry.Count() == 1 && initMsg.Protocol == "simple" {
				c.mode = modeSimple
			}
			if err := c.sendStatus(nil, initMsg.Tag); err != nil {
				return err
			}
		case protocol.MessageInitVDC:
			b, _ := json.Marshal(obj)
			var iv protocol.InitVDCMessage
			if err := json.Unmarshal(b, &iv); err != nil {
				return fmt.Errorf("invalid initvdc message: %w", err)
			}
			c.vdcInfo = iv
		case "log":
			level, _ := obj["level"].(float64)
			text, _ := obj["text"].(string)
			if text != "" {
				log.Printf("device log level=%d msg=%s", int(level), text)
			}
		case "bye":
			tag, _ := obj["tag"].(string)
			d, err := c.registry.ResolveTag(tag)
			if err != nil {
				return withStatusTag(tag, err)
			}
			c.registry.Remove(d.Tag)
			c.emitEvent(runtime.Event{Type: runtime.EventRemove, Tag: d.Tag, UniqueID: d.UniqueID})
		default:
			if err := c.handleDeviceJSON(obj); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Connector) handleDeviceJSON(obj map[string]any) error {
	msg, _ := obj["message"].(string)
	tag, _ := obj["tag"].(string)
	dev, err := c.registry.ResolveTag(tag)
	if err != nil {
		return withStatusTag(tag, err)
	}
	if err := protocol.ValidateDeviceMessageJSON(msg, obj); err != nil {
		return withStatusTag(tag, err)
	}

	switch msg {
	case "channel":
		if idx, val, ok := indexAndValue(obj); ok {
			c.registry.SetChannel(tag, idx, val)
			c.emitEvent(runtime.Event{Type: runtime.EventChannel, Tag: dev.Tag, UniqueID: dev.UniqueID, Index: idx, Value: val})
		}
	case "button":
		if idx, val, ok := indexAndValue(obj); ok {
			mode, _ := obj["mode"].(string)
			c.registry.SetButton(tag, idx, val)
			c.emitEvent(runtime.Event{Type: runtime.EventButton, Tag: dev.Tag, UniqueID: dev.UniqueID, Index: idx, Value: val})
			c.emitButtonAction(dev.Tag, dev.UniqueID, idx, val, mode)
		}
	case "input":
		if idx, val, ok := indexAndValue(obj); ok {
			c.registry.SetInput(tag, idx, val)
			c.emitEvent(runtime.Event{Type: runtime.EventInput, Tag: dev.Tag, UniqueID: dev.UniqueID, Index: idx, Value: val})
		}
	case "sensor":
		if idx, val, ok := indexAndValue(obj); ok {
			c.registry.SetSensor(tag, idx, val)
			c.emitEvent(runtime.Event{Type: runtime.EventSensor, Tag: dev.Tag, UniqueID: dev.UniqueID, Index: idx, Value: val})
		}
	case "sync":
		c.registry.SetSync(tag, true)
	case "synced":
		c.registry.SetSync(tag, false)
	case "active":
		if b, ok := boolValue(obj["value"]); ok {
			c.registry.SetActive(tag, b)
			c.emitEvent(runtime.Event{Type: runtime.EventActive, Tag: dev.Tag, UniqueID: dev.UniqueID, Active: b})
		}
	case "opstate":
		var level *int
		if lv, ok := obj["level"].(float64); ok {
			i := int(lv)
			level = &i
		}
		var text *string
		if tv, ok := obj["text"].(string); ok {
			text = &tv
		}
		c.registry.SetOpState(tag, level, text)
	}
	return nil
}

func indexAndValue(obj map[string]any) (int, float64, bool) {
	i, iok := obj["index"].(float64)
	v, vok := obj["value"].(float64)
	if !iok || !vok {
		return 0, 0, false
	}
	return int(i), v, true
}

func boolValue(v any) (bool, bool) {
	if b, ok := v.(bool); ok {
		return b, true
	}
	if f, ok := v.(float64); ok {
		return f != 0, true
	}
	return false, false
}
