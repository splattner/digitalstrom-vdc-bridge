// Package mqtt provides a shared MQTT broker service used by bridge plugins.
//
// The lifecycle of a broker connection is owned by the "mqtt" bridge plugin
// (pkg/bridge/mqtt), which registers each broker instance with a Manager at
// Init time. Other plugins (e.g. Tasmota, Zigbee2MQTT, MQTT-discovery) look
// up brokers by id via Manager.Get and call Publish / Subscribe directly.
package mqtt

import (
	"context"
	"errors"
	"sync"
)

// MessageHandler is invoked for every received message on a topic subscription.
type MessageHandler func(topic string, payload []byte, retained bool)

// Subscription represents one active topic subscription. Calling Close
// unsubscribes from the broker.
type Subscription interface {
	// Topic returns the subscribed topic filter.
	Topic() string
	// Close unsubscribes and releases resources. Idempotent.
	Close() error
}

// Broker is the interface exposed by an MQTT broker connection. Implementations
// live in pkg/bridge/mqtt; consumers obtain a Broker via Manager.Get.
type Broker interface {
	// ID returns the broker instance id (matches the MQTT plugin instance id).
	ID() string
	// Status returns a short, human-readable connection status
	// (e.g. "connected", "reconnecting", "auth_failed").
	Status() string
	// Connected reports whether the broker is currently connected.
	Connected() bool
	// Publish sends a message. If qos>0 the call returns once the broker
	// acknowledges the publish.
	Publish(ctx context.Context, topic string, payload []byte, qos byte, retain bool) error
	// Subscribe installs a topic-filter subscription. The returned Subscription
	// must be closed to unsubscribe.
	Subscribe(topic string, qos byte, handler MessageHandler) (Subscription, error)
	// OnConnect registers a callback fired every time the broker (re)connects.
	// The callback is invoked once immediately if already connected.
	OnConnect(func())
	// OnDisconnect registers a callback fired every time the broker disconnects.
	OnDisconnect(func(error))
}

// ErrNotFound is returned by Manager.Get when no broker is registered under
// the requested id.
var ErrNotFound = errors.New("mqtt: broker not found")

// Manager is the registry of active MQTT brokers.
type Manager struct {
	mu      sync.RWMutex
	brokers map[string]Broker
	// listeners are notified whenever a broker is registered or unregistered
	listeners []func(id string, b Broker, registered bool)
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{brokers: make(map[string]Broker)}
}

// Register installs (or replaces) a broker under id. Listeners are notified.
// The MQTT bridge plugin calls this from its Init.
func (m *Manager) Register(id string, b Broker) {
	m.mu.Lock()
	m.brokers[id] = b
	listeners := append([]func(string, Broker, bool){}, m.listeners...)
	m.mu.Unlock()
	for _, l := range listeners {
		l(id, b, true)
	}
}

// Unregister removes the broker under id, if any. Listeners are notified.
func (m *Manager) Unregister(id string) {
	m.mu.Lock()
	b, ok := m.brokers[id]
	delete(m.brokers, id)
	listeners := append([]func(string, Broker, bool){}, m.listeners...)
	m.mu.Unlock()
	if ok {
		for _, l := range listeners {
			l(id, b, false)
		}
	}
}

// Get returns the broker registered under id.
func (m *Manager) Get(id string) (Broker, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.brokers[id]
	if !ok {
		return nil, ErrNotFound
	}
	return b, nil
}

// IDs returns the list of currently registered broker ids.
func (m *Manager) IDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.brokers))
	for id := range m.brokers {
		out = append(out, id)
	}
	return out
}

// Subscribe installs a listener for register / unregister events. Useful for
// plugins that want to react to broker availability changes.
func (m *Manager) AddListener(l func(id string, b Broker, registered bool)) {
	m.mu.Lock()
	m.listeners = append(m.listeners, l)
	m.mu.Unlock()
}
