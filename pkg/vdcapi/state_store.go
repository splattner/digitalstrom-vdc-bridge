package vdcapi

import (
	"strings"
	"sync"
	"time"

	"github.com/splattner/vdcgo/pkg/runtime"
)

type ExternalDeviceState struct {
	Key                    string
	UniqueID               string
	Name                   string
	Output                 string
	Channels               map[int]float64
	ChannelUpdatedAt       map[int]time.Time
	Buttons                map[int]float64
	ButtonUpdatedAt        map[int]time.Time
	ButtonActions          map[int]string
	Inputs                 map[int]float64
	InputUpdatedAt         map[int]time.Time
	Sensors                map[int]float64
	SensorUpdatedAt        map[int]time.Time
	SensorDescriptors      map[int]runtime.SensorDescriptor
	BinaryInputDescriptors map[int]runtime.BinaryInputDescriptor
	Active                 bool
}

type ExternalSnapshot struct {
	Devices map[string]ExternalDeviceState
}

type StateUpdate struct {
	Type   string
	Device ExternalDeviceState
}

// StateStore tracks external light devices mirrored to vDC API.
type StateStore struct {
	mu          sync.RWMutex
	devices     map[string]ExternalDeviceState
	tagIndex    map[string]string
	nextSubID   int
	subscribers map[int]chan StateUpdate
}

func NewStateStore() *StateStore {
	return &StateStore{
		devices:     make(map[string]ExternalDeviceState),
		tagIndex:    make(map[string]string),
		subscribers: make(map[int]chan StateUpdate),
	}
}

func (s *StateStore) Snapshot() ExternalSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := ExternalSnapshot{Devices: make(map[string]ExternalDeviceState, len(s.devices))}
	for k, d := range s.devices {
		out.Devices[k] = d
	}
	return out
}

func (s *StateStore) HandleEvent(e runtime.Event) {
	var update *StateUpdate

	s.mu.Lock()
	switch e.Type {
	case runtime.EventInit:
		key := eventKey(e)
		d := s.devices[key]
		d.Key = key
		d.UniqueID = strings.TrimSpace(e.UniqueID)
		d.Output = strings.TrimSpace(e.Output)
		if d.Output == "" {
			d.Output = "light"
		}
		if d.Buttons == nil {
			d.Buttons = make(map[int]float64)
		}
		if d.Channels == nil {
			d.Channels = make(map[int]float64)
		}
		if d.ButtonActions == nil {
			d.ButtonActions = make(map[int]string)
		}
		if d.Inputs == nil {
			d.Inputs = make(map[int]float64)
		}
		if d.Sensors == nil {
			d.Sensors = make(map[int]float64)
		}
		if strings.TrimSpace(e.Name) != "" {
			d.Name = strings.TrimSpace(e.Name)
		}
		if d.Name == "" {
			d.Name = d.UniqueID
		}
		d.Active = true
		s.devices[key] = d
		if e.Tag != "" {
			s.tagIndex[connectionTag(e.Connection, e.Tag)] = key
		}
		u := StateUpdate{Type: runtime.EventInit, Device: d}
		update = &u
	case runtime.EventRemove:
		if key, ok := s.resolveKey(e); ok {
			d := s.devices[key]
			delete(s.devices, key)
			if e.Tag != "" {
				delete(s.tagIndex, connectionTag(e.Connection, e.Tag))
			}
			u := StateUpdate{Type: runtime.EventRemove, Device: d}
			update = &u
		}
	case runtime.EventChannel:
		if key, ok := s.resolveKey(e); ok {
			d := s.devices[key]
			if d.Channels == nil {
				d.Channels = make(map[int]float64)
			}
			if d.ChannelUpdatedAt == nil {
				d.ChannelUpdatedAt = make(map[int]time.Time)
			}
			d.Channels[e.Index] = e.Value
			d.ChannelUpdatedAt[e.Index] = time.Now()
			s.devices[key] = d
			u := StateUpdate{Type: runtime.EventChannel, Device: d}
			update = &u
		}
	case runtime.EventButton:
		if key, ok := s.resolveKey(e); ok {
			d := s.devices[key]
			if d.Buttons == nil {
				d.Buttons = make(map[int]float64)
			}
			if d.ButtonUpdatedAt == nil {
				d.ButtonUpdatedAt = make(map[int]time.Time)
			}
			d.Buttons[e.Index] = e.Value
			d.ButtonUpdatedAt[e.Index] = time.Now()
			s.devices[key] = d
			u := StateUpdate{Type: runtime.EventButton, Device: d}
			update = &u
		}
	case runtime.EventButtonAction:
		if key, ok := s.resolveKey(e); ok {
			d := s.devices[key]
			if d.ButtonActions == nil {
				d.ButtonActions = make(map[int]string)
			}
			d.ButtonActions[e.Index] = strings.TrimSpace(e.Action)
			s.devices[key] = d
			u := StateUpdate{Type: runtime.EventButtonAction, Device: d}
			update = &u
		}
	case runtime.EventInput:
		if key, ok := s.resolveKey(e); ok {
			d := s.devices[key]
			if d.Inputs == nil {
				d.Inputs = make(map[int]float64)
			}
			if d.InputUpdatedAt == nil {
				d.InputUpdatedAt = make(map[int]time.Time)
			}
			d.Inputs[e.Index] = e.Value
			d.InputUpdatedAt[e.Index] = time.Now()
			s.devices[key] = d
			u := StateUpdate{Type: runtime.EventInput, Device: d}
			update = &u
		}
	case runtime.EventSensor:
		if key, ok := s.resolveKey(e); ok {
			d := s.devices[key]
			if d.Sensors == nil {
				d.Sensors = make(map[int]float64)
			}
			if d.SensorUpdatedAt == nil {
				d.SensorUpdatedAt = make(map[int]time.Time)
			}
			d.Sensors[e.Index] = e.Value
			d.SensorUpdatedAt[e.Index] = time.Now()
			s.devices[key] = d
			u := StateUpdate{Type: runtime.EventSensor, Device: d}
			update = &u
		}
	case runtime.EventSensorDescriptor:
		if e.SensorDescriptor == nil {
			break
		}
		if key, ok := s.resolveKey(e); ok {
			d := s.devices[key]
			if d.SensorDescriptors == nil {
				d.SensorDescriptors = make(map[int]runtime.SensorDescriptor)
			}
			d.SensorDescriptors[e.Index] = *e.SensorDescriptor
			s.devices[key] = d
			// No push to vDSM needed: descriptors are read on next getProperty.
		}
	case runtime.EventBinaryInputDescriptor:
		if e.BinaryInputDescriptor == nil {
			break
		}
		if key, ok := s.resolveKey(e); ok {
			d := s.devices[key]
			if d.BinaryInputDescriptors == nil {
				d.BinaryInputDescriptors = make(map[int]runtime.BinaryInputDescriptor)
			}
			d.BinaryInputDescriptors[e.Index] = *e.BinaryInputDescriptor
			s.devices[key] = d
		}
	case runtime.EventActive:
		if key, ok := s.resolveKey(e); ok {
			d := s.devices[key]
			d.Active = e.Active
			s.devices[key] = d
			u := StateUpdate{Type: runtime.EventActive, Device: d}
			update = &u
		}
	}
	s.mu.Unlock()

	if update != nil {
		s.broadcast(*update)
	}
}

func (s *StateStore) Subscribe() (id int, ch <-chan StateUpdate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = s.nextSubID
	s.nextSubID++
	c := make(chan StateUpdate, 16)
	s.subscribers[id] = c
	return id, c
}

func (s *StateStore) Unsubscribe(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.subscribers[id]; ok {
		delete(s.subscribers, id)
		close(c)
	}
}

func (s *StateStore) resolveKey(e runtime.Event) (string, bool) {
	uid := strings.TrimSpace(e.UniqueID)
	if uid != "" {
		key := "uid:" + uid
		_, ok := s.devices[key]
		return key, ok
	}
	if e.Tag == "" {
		return "", false
	}
	key, ok := s.tagIndex[connectionTag(e.Connection, e.Tag)]
	if !ok {
		return "", false
	}
	_, exists := s.devices[key]
	return key, exists
}

func eventKey(e runtime.Event) string {
	uid := strings.TrimSpace(e.UniqueID)
	if uid != "" {
		return "uid:" + uid
	}
	if e.Connection == "" && e.Tag == "" {
		return "anon:sample"
	}
	return "conn:" + connectionTag(e.Connection, e.Tag)
}

func connectionTag(conn, tag string) string {
	return strings.TrimSpace(conn) + "|" + strings.TrimSpace(tag)
}

// ReAnnounce broadcasts a StateUpdate{EventInit} for the device identified by
// uniqueID. This triggers the pbuf_server to re-send Vanish + AnnounceDevice +
// PushNotification so that vdcd/dSS re-queries all device properties (including
// sensor descriptions). Call this after pushing descriptors that were not yet
// available when the device was first announced.
func (s *StateStore) ReAnnounce(uniqueID string) {
	uniqueID = strings.TrimSpace(uniqueID)
	if uniqueID == "" {
		return
	}
	key := "uid:" + uniqueID
	s.mu.RLock()
	d, ok := s.devices[key]
	s.mu.RUnlock()
	if !ok {
		return
	}
	s.broadcast(StateUpdate{Type: runtime.EventInit, Device: d})
}

func (s *StateStore) broadcast(update StateUpdate) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.subscribers {
		select {
		case c <- update:
		default:
		}
	}
}
