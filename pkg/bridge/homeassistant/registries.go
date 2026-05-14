package homeassistant

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/coder/websocket"
)

// haArea is one entry from `config/area_registry/list`.
type haArea struct {
	AreaID string `json:"area_id"`
	Name   string `json:"name"`
}

// haDevice is one entry from `config/device_registry/list`. Only the fields
// we surface to the UI are decoded.
type haDevice struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	NameByUser   string `json:"name_by_user"`
	AreaID       string `json:"area_id"`
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
}

// haEntityReg is one entry from `config/entity_registry/list`.
type haEntityReg struct {
	EntityID string `json:"entity_id"`
	DeviceID string `json:"device_id"`
	AreaID   string `json:"area_id"`
	Name     string `json:"name"`
}

// haRegistries holds the resolved HA registry data, keyed for fast lookup.
type haRegistries struct {
	Areas    map[string]haArea      // area_id → area
	Devices  map[string]haDevice    // device_id → device
	Entities map[string]haEntityReg // entity_id → entity registry entry
}

// fetchRegistries loads area, device, entity registries from the connected HA
// instance. Best-effort: if any one call fails it returns what it has plus an
// error (the caller may still publish partial data).
func (c *wsClient) fetchRegistries(ctx context.Context, conn *websocket.Conn) (haRegistries, error) {
	regs := haRegistries{
		Areas:    map[string]haArea{},
		Devices:  map[string]haDevice{},
		Entities: map[string]haEntityReg{},
	}

	// Areas
	if raw, err := c.command(ctx, conn, map[string]any{"type": "config/area_registry/list"}); err == nil {
		var areas []haArea
		if err := decodeListResult(raw, &areas); err == nil {
			for _, a := range areas {
				regs.Areas[a.AreaID] = a
			}
		}
	} else {
		return regs, fmt.Errorf("area_registry: %w", err)
	}

	// Devices
	if raw, err := c.command(ctx, conn, map[string]any{"type": "config/device_registry/list"}); err == nil {
		var devs []haDevice
		if err := decodeListResult(raw, &devs); err == nil {
			for _, d := range devs {
				regs.Devices[d.ID] = d
			}
		}
	} else {
		return regs, fmt.Errorf("device_registry: %w", err)
	}

	// Entities
	if raw, err := c.command(ctx, conn, map[string]any{"type": "config/entity_registry/list"}); err == nil {
		var ents []haEntityReg
		if err := decodeListResult(raw, &ents); err == nil {
			for _, e := range ents {
				regs.Entities[e.EntityID] = e
			}
		}
	} else {
		return regs, fmt.Errorf("entity_registry: %w", err)
	}

	return regs, nil
}

// command sends a typed command to HA and waits for the matching `result`.
// The returned bytes are the full result envelope (see decodeListResult).
func (c *wsClient) command(ctx context.Context, conn *websocket.Conn, msg map[string]any) (json.RawMessage, error) {
	id := c.nextMsgID()
	ch := c.registerPending(id)
	defer c.cancelPending(id)
	msg["id"] = id
	if err := c.send(ctx, conn, msg); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case raw := <-ch:
		return raw, nil
	}
}

// decodeListResult decodes the `result` field of a generic command envelope
// into the given destination (typically a slice).
func decodeListResult(raw json.RawMessage, dst any) error {
	var env struct {
		Success bool            `json:"success"`
		Result  json.RawMessage `json:"result"`
		Error   any             `json:"error,omitempty"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return err
	}
	if !env.Success {
		return fmt.Errorf("command failed: %v", env.Error)
	}
	if len(env.Result) == 0 {
		return nil
	}
	return json.Unmarshal(env.Result, dst)
}

// resolveExtras returns plugin-specific attributes for an entity (device name,
// area name, manufacturer, model). Fields that resolve to empty are omitted.
func (r haRegistries) resolveExtras(entityID string) map[string]any {
	out := map[string]any{}
	ent, ok := r.Entities[entityID]
	areaID := ""
	if ok {
		areaID = ent.AreaID
		if dev, ok := r.Devices[ent.DeviceID]; ok {
			devName := dev.NameByUser
			if devName == "" {
				devName = dev.Name
			}
			if devName != "" {
				out["device"] = devName
			}
			if dev.Manufacturer != "" {
				out["manufacturer"] = dev.Manufacturer
			}
			if dev.Model != "" {
				out["model"] = dev.Model
			}
			if areaID == "" {
				// Entities inherit their device's area when none is set explicitly.
				areaID = dev.AreaID
			}
		}
	}
	if areaID != "" {
		if a, ok := r.Areas[areaID]; ok && a.Name != "" {
			out["area"] = a.Name
		}
	}
	return out
}
