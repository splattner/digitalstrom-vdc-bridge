package runtime

import (
	"fmt"
	"sync"
)

// Device is a connection-local external device registration.
type Device struct {
	Tag      string
	UniqueID string
	Output   string
	Name     string
}

// DeviceState contains the latest runtime values observed from a device.
type DeviceState struct {
	Channels map[int]float64
	Buttons  map[int]float64
	Inputs   map[int]float64
	Sensors  map[int]float64

	SyncInProgress bool
	Active         bool
	OpStateLevel   int
	OpStateText    string
}

// Registry tracks devices per connection.
type Registry struct {
	mu      sync.RWMutex
	devices map[string]Device
	states  map[string]*DeviceState
}

func NewRegistry() *Registry {
	return &Registry{
		devices: make(map[string]Device),
		states:  make(map[string]*DeviceState),
	}
}

func (r *Registry) Add(d Device) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.devices[d.Tag]; exists {
		return fmt.Errorf("device with tag %q already exists", d.Tag)
	}
	r.devices[d.Tag] = d
	r.states[d.Tag] = &DeviceState{
		Channels: make(map[int]float64),
		Buttons:  make(map[int]float64),
		Inputs:   make(map[int]float64),
		Sensors:  make(map[int]float64),
	}
	return nil
}

func (r *Registry) Remove(tag string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.devices, tag)
	delete(r.states, tag)
}

func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.devices)
}

func (r *Registry) ResolveTag(tag string) (Device, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if tag != "" {
		d, ok := r.devices[tag]
		if !ok {
			return Device{}, fmt.Errorf("no device tagged %q found", tag)
		}
		return d, nil
	}
	if len(r.devices) == 1 {
		for _, d := range r.devices {
			return d, nil
		}
	}
	if len(r.devices) > 1 {
		return Device{}, fmt.Errorf("missing tag")
	}
	return Device{}, fmt.Errorf("no registered device")
}

func (r *Registry) FindByUniqueID(uniqueID string) (Device, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, d := range r.devices {
		if d.UniqueID == uniqueID {
			return d, true
		}
	}
	return Device{}, false
}

func (r *Registry) SetSync(tag string, inProgress bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if st := r.states[tag]; st != nil {
		st.SyncInProgress = inProgress
	}
}

func (r *Registry) SetActive(tag string, active bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if st := r.states[tag]; st != nil {
		st.Active = active
	}
}

func (r *Registry) SetOpState(tag string, level *int, text *string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if st := r.states[tag]; st != nil {
		if level != nil {
			st.OpStateLevel = *level
		}
		if text != nil {
			st.OpStateText = *text
		}
	}
}

func (r *Registry) SetChannel(tag string, index int, value float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if st := r.states[tag]; st != nil {
		st.Channels[index] = value
	}
}

func (r *Registry) SetButton(tag string, index int, value float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if st := r.states[tag]; st != nil {
		st.Buttons[index] = value
	}
}

func (r *Registry) SetInput(tag string, index int, value float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if st := r.states[tag]; st != nil {
		st.Inputs[index] = value
	}
}

func (r *Registry) SetSensor(tag string, index int, value float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if st := r.states[tag]; st != nil {
		st.Sensors[index] = value
	}
}

// AllDevices returns a snapshot of all currently-registered devices.
func (r *Registry) AllDevices() []Device {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Device, 0, len(r.devices))
	for _, d := range r.devices {
		out = append(out, d)
	}
	return out
}

func (r *Registry) Snapshot(tag string) (DeviceState, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	st := r.states[tag]
	if st == nil {
		return DeviceState{}, false
	}
	copyState := DeviceState{
		Channels:       make(map[int]float64, len(st.Channels)),
		Buttons:        make(map[int]float64, len(st.Buttons)),
		Inputs:         make(map[int]float64, len(st.Inputs)),
		Sensors:        make(map[int]float64, len(st.Sensors)),
		SyncInProgress: st.SyncInProgress,
		Active:         st.Active,
		OpStateLevel:   st.OpStateLevel,
		OpStateText:    st.OpStateText,
	}
	for k, v := range st.Channels {
		copyState.Channels[k] = v
	}
	for k, v := range st.Buttons {
		copyState.Buttons[k] = v
	}
	for k, v := range st.Inputs {
		copyState.Inputs[k] = v
	}
	for k, v := range st.Sensors {
		copyState.Sensors[k] = v
	}
	return copyState, true
}
