// Package mqtt is the bridge plugin that owns MQTT broker connections and
// registers them with pkg/services/mqtt.Manager so other plugins can use them.
//
// The plugin itself does not expose any vDC devices; Discover always returns
// an empty list and Subscribe / Apply are no-ops.
package mqtt

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"github.com/splattner/vdcgo/pkg/bridge"
	"github.com/splattner/vdcgo/pkg/logging"
	mqttsvc "github.com/splattner/vdcgo/pkg/services/mqtt"
)

// PluginType is the registered Factory key for MQTT plugins.
const PluginType = "mqtt"

// Factory returns a bridge.Factory that produces MQTT plugin instances.
func Factory() bridge.Factory {
	return func(id string) bridge.Plugin {
		return &Plugin{id: id}
	}
}

// Plugin is the bridge.Plugin implementation that owns one MQTT broker.
type Plugin struct {
	id      string
	broker  *brokerImpl
	manager *mqttsvc.Manager
	closed  atomic.Bool
}

// ID returns the plugin instance id.
func (p *Plugin) ID() string { return p.id }

// Status reflects the underlying broker connection state.
func (p *Plugin) Status() string {
	if p.broker == nil {
		return "not_initialized"
	}
	return p.broker.Status()
}

// Init parses config, builds an MQTT client, and registers the broker.
func (p *Plugin) Init(ctx context.Context, cfg map[string]any, host bridge.Host) error {
	c, err := parseConfig(cfg)
	if err != nil {
		return fmt.Errorf("mqtt %q: %w", p.id, err)
	}
	p.manager = host.MQTT()

	b := newBroker(p.id, c)
	p.broker = b
	if p.manager != nil {
		p.manager.Register(p.id, b)
	}

	b.OnConnect(func() {
		host.Log(bridge.LevelInfo, bridge.CodeConnectOK, "MQTT broker connected",
			map[string]any{"host": c.host, "port": c.port})
	})
	b.OnDisconnect(func(err error) {
		host.Log(bridge.LevelWarn, bridge.CodeConnectFailed, "MQTT broker disconnected",
			map[string]any{"host": c.host, "error": err.Error()})
	})
	b.OnConnectError(func(err error) {
		host.Log(bridge.LevelError, bridge.CodeConnectFailed, "MQTT initial connect failed",
			map[string]any{"host": c.host, "port": c.port, "error": err.Error()})
	})

	go b.run(ctx)
	logging.Info("mqtt_plugin_started", logging.Fields{
		"id":     p.id,
		"host":   c.host,
		"port":   c.port,
		"tls":    c.tls,
		"client": c.clientID,
	})
	return nil
}

// Discover returns no entities — MQTT is a helper plugin.
func (p *Plugin) Discover(_ context.Context) ([]bridge.RemoteEntity, error) {
	return []bridge.RemoteEntity{}, nil
}

// Subscribe is a no-op (plugin owns no devices itself).
func (p *Plugin) Subscribe(_ context.Context, _ bridge.Mapping) error { return nil }

// Unsubscribe is a no-op.
func (p *Plugin) Unsubscribe(_ context.Context, _ string) error { return nil }

// Apply is a no-op (no devices to control).
func (p *Plugin) Apply(_ context.Context, _ bridge.Mapping, _ bridge.Command) error {
	return errors.New("mqtt plugin has no own devices")
}

// Close disconnects the broker and unregisters it from the manager.
func (p *Plugin) Close() error {
	if !p.closed.CompareAndSwap(false, true) {
		return nil
	}
	if p.manager != nil {
		p.manager.Unregister(p.id)
	}
	if p.broker != nil {
		p.broker.stop()
	}
	return nil
}

// ── broker implementation ─────────────────────────────────────────────────────

// brokerImpl wraps a paho client and implements mqttsvc.Broker.
type brokerImpl struct {
	id  string
	cfg config

	mu        sync.RWMutex
	client    paho.Client
	status    string
	lastErr   error
	connected atomic.Bool
	subs      map[*subscriptionImpl]struct{}
	// topicSubs lists the active subscriptions per topic filter so that
	// multiple consumers of the same topic can fan out from a single Paho
	// subscription. Paho only allows one handler per topic; without this
	// fan-out, a second Subscribe on the same topic silently displaces the
	// first (was the cause of multi-button z2m devices losing all but the
	// last bridged button).
	topicSubs    map[string][]*subscriptionImpl
	onConnect    []func()
	onDisconnect []func(error)
	onConnectErr []func(error)

	cancel context.CancelFunc
}

func newBroker(id string, c config) *brokerImpl {
	return &brokerImpl{
		id:        id,
		cfg:       c,
		status:    "starting",
		subs:      make(map[*subscriptionImpl]struct{}),
		topicSubs: make(map[string][]*subscriptionImpl),
	}
}

func (b *brokerImpl) ID() string { return b.id }

func (b *brokerImpl) Status() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.status
}

func (b *brokerImpl) Connected() bool { return b.connected.Load() }

// run builds the paho client and lets it manage its own reconnect loop.
// Returns when ctx is cancelled.
func (b *brokerImpl) run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	b.mu.Lock()
	b.cancel = cancel
	b.mu.Unlock()

	opts := paho.NewClientOptions()
	scheme := "tcp"
	if b.cfg.tls {
		scheme = "tls"
	}
	opts.AddBroker(fmt.Sprintf("%s://%s:%d", scheme, b.cfg.host, b.cfg.port))
	opts.SetClientID(b.cfg.clientID)
	opts.SetCleanSession(b.cfg.cleanSession)
	opts.SetKeepAlive(time.Duration(b.cfg.keepalive) * time.Second)
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(60 * time.Second)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)
	opts.SetOrderMatters(false)

	if b.cfg.username != "" {
		opts.SetUsername(b.cfg.username)
		opts.SetPassword(b.cfg.password)
	}
	if b.cfg.tls {
		tlsCfg := &tls.Config{InsecureSkipVerify: b.cfg.tlsInsecure}
		if b.cfg.caCert != "" {
			pool := x509.NewCertPool()
			if pool.AppendCertsFromPEM([]byte(b.cfg.caCert)) {
				tlsCfg.RootCAs = pool
			} else {
				logging.Warn("mqtt_ca_invalid", logging.Fields{"id": b.id})
			}
		}
		opts.SetTLSConfig(tlsCfg)
	}
	if b.cfg.will != nil {
		opts.SetWill(b.cfg.will.topic, b.cfg.will.payload, b.cfg.will.qos, b.cfg.will.retain)
	}

	opts.SetOnConnectHandler(func(_ paho.Client) {
		b.connected.Store(true)
		b.setStatus("connected", nil)
		logging.Info("mqtt_connected", logging.Fields{"id": b.id, "host": b.cfg.host})

		// Re-install one Paho subscription per distinct topic on (re)connect.
		// All locally registered handlers for that topic share the same Paho
		// subscription via the fan-out dispatcher below.
		b.mu.RLock()
		topics := make(map[string]byte, len(b.topicSubs))
		for topic, list := range b.topicSubs {
			var maxQos byte
			for _, s := range list {
				if s.qos > maxQos {
					maxQos = s.qos
				}
			}
			topics[topic] = maxQos
		}
		cbs := append([]func(){}, b.onConnect...)
		b.mu.RUnlock()
		for topic, qos := range topics {
			if err := b.installTopic(topic, qos); err != nil {
				logging.Warn("mqtt_resubscribe_error", logging.Fields{"id": b.id, "topic": topic, "error": err.Error()})
			}
		}
		for _, cb := range cbs {
			go cb()
		}
	})
	opts.SetConnectionLostHandler(func(_ paho.Client, err error) {
		b.connected.Store(false)
		b.setStatus("reconnecting", err)
		logging.Warn("mqtt_disconnected", logging.Fields{"id": b.id, "error": err.Error()})
		b.mu.RLock()
		cbs := append([]func(error){}, b.onDisconnect...)
		b.mu.RUnlock()
		for _, cb := range cbs {
			go cb(err)
		}
	})
	opts.SetReconnectingHandler(func(_ paho.Client, _ *paho.ClientOptions) {
		b.setStatus("reconnecting", nil)
	})

	client := paho.NewClient(opts)
	b.mu.Lock()
	b.client = client
	b.mu.Unlock()

	b.setStatus("connecting", nil)
	tok := client.Connect()
	// Don't block forever — paho will keep retrying via SetConnectRetry.
	go func() {
		if !tok.WaitTimeout(30 * time.Second) {
			err := errors.New("initial connect timeout")
			b.setStatus("connecting", err)
			b.fireConnectError(err)
			return
		}
		if err := tok.Error(); err != nil {
			b.setStatus("auth_failed", err)
			logging.Warn("mqtt_connect_error", logging.Fields{"id": b.id, "error": err.Error()})
			b.fireConnectError(err)
		}
	}()

	<-ctx.Done()
	if client.IsConnected() {
		client.Disconnect(500)
	}
	b.connected.Store(false)
	b.setStatus("stopped", nil)
}

func (b *brokerImpl) stop() {
	b.mu.Lock()
	c := b.cancel
	b.mu.Unlock()
	if c != nil {
		c()
	}
}

func (b *brokerImpl) setStatus(s string, err error) {
	b.mu.Lock()
	b.status = s
	b.lastErr = err
	b.mu.Unlock()
}

// Publish implements mqttsvc.Broker.Publish.
func (b *brokerImpl) Publish(ctx context.Context, topic string, payload []byte, qos byte, retain bool) error {
	b.mu.RLock()
	c := b.client
	b.mu.RUnlock()
	if c == nil {
		return errors.New("mqtt: not initialised")
	}
	tok := c.Publish(topic, qos, retain, payload)
	done := make(chan error, 1)
	go func() {
		tok.Wait()
		done <- tok.Error()
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

// Subscribe implements mqttsvc.Broker.Subscribe. Multiple subscribers may
// register handlers for the same topic; all are dispatched on every received
// message (Paho itself only retains one handler per topic, so we fan out
// here).
func (b *brokerImpl) Subscribe(topic string, qos byte, h mqttsvc.MessageHandler) (mqttsvc.Subscription, error) {
	if h == nil {
		return nil, errors.New("mqtt: handler is nil")
	}
	s := &subscriptionImpl{broker: b, topic: topic, qos: qos, handler: h}
	b.mu.Lock()
	b.subs[s] = struct{}{}
	list := b.topicSubs[topic]
	first := len(list) == 0
	b.topicSubs[topic] = append(list, s)
	connected := b.connected.Load()
	b.mu.Unlock()

	if connected && first {
		if err := b.installTopic(topic, qos); err != nil {
			b.mu.Lock()
			delete(b.subs, s)
			b.topicSubs[topic] = removeSub(b.topicSubs[topic], s)
			if len(b.topicSubs[topic]) == 0 {
				delete(b.topicSubs, topic)
			}
			b.mu.Unlock()
			return nil, err
		}
	}
	return s, nil
}

// installTopic installs a single Paho subscription for the topic. The Paho
// callback dispatches to every locally registered handler for that topic.
func (b *brokerImpl) installTopic(topic string, qos byte) error {
	b.mu.RLock()
	c := b.client
	b.mu.RUnlock()
	if c == nil {
		return errors.New("mqtt: not initialised")
	}
	tok := c.Subscribe(topic, qos, func(_ paho.Client, msg paho.Message) {
		b.dispatch(msg.Topic(), msg.Payload(), msg.Retained())
	})
	if !tok.WaitTimeout(10 * time.Second) {
		return errors.New("mqtt: subscribe timeout")
	}
	return tok.Error()
}

// dispatch fans a received message out to every active handler whose
// subscribed topic matches the (possibly wildcard) filter.
func (b *brokerImpl) dispatch(topic string, payload []byte, retained bool) {
	b.mu.RLock()
	handlers := make([]mqttsvc.MessageHandler, 0)
	for filter, list := range b.topicSubs {
		if !topicMatches(filter, topic) {
			continue
		}
		for _, s := range list {
			if s.closed.Load() {
				continue
			}
			handlers = append(handlers, s.handler)
		}
	}
	b.mu.RUnlock()
	for _, h := range handlers {
		h(topic, payload, retained)
	}
}

// removeSub returns list with s removed (in-place when possible).
func removeSub(list []*subscriptionImpl, s *subscriptionImpl) []*subscriptionImpl {
	for i, x := range list {
		if x == s {
			return append(list[:i], list[i+1:]...)
		}
	}
	return list
}

// topicMatches reports whether an MQTT topic filter matches a concrete topic.
// Supports the standard wildcards: '+' (single level) and '#' (multi level,
// must be the last segment).
func topicMatches(filter, topic string) bool {
	if filter == topic {
		return true
	}
	fp := splitTopic(filter)
	tp := splitTopic(topic)
	for i, seg := range fp {
		if seg == "#" {
			return true
		}
		if i >= len(tp) {
			return false
		}
		if seg == "+" {
			continue
		}
		if seg != tp[i] {
			return false
		}
	}
	return len(fp) == len(tp)
}

func splitTopic(t string) []string {
	if t == "" {
		return nil
	}
	return strings.Split(t, "/")
}

// OnConnect registers a callback fired after every successful connect.
func (b *brokerImpl) OnConnect(cb func()) {
	if cb == nil {
		return
	}
	b.mu.Lock()
	b.onConnect = append(b.onConnect, cb)
	connected := b.connected.Load()
	b.mu.Unlock()
	if connected {
		go cb()
	}
}

// OnDisconnect registers a callback fired after every disconnect.
func (b *brokerImpl) OnDisconnect(cb func(error)) {
	if cb == nil {
		return
	}
	b.mu.Lock()
	b.onDisconnect = append(b.onDisconnect, cb)
	b.mu.Unlock()
}

// OnConnectError registers a callback fired when the initial connect attempt
// times out or the broker rejects the credentials. Unlike OnDisconnect this
// does NOT fire on every later transient reconnect.
func (b *brokerImpl) OnConnectError(cb func(error)) {
	if cb == nil {
		return
	}
	b.mu.Lock()
	b.onConnectErr = append(b.onConnectErr, cb)
	b.mu.Unlock()
}

func (b *brokerImpl) fireConnectError(err error) {
	b.mu.RLock()
	cbs := append([]func(error){}, b.onConnectErr...)
	b.mu.RUnlock()
	for _, cb := range cbs {
		go cb(err)
	}
}

// subscriptionImpl is one active topic subscription.
type subscriptionImpl struct {
	broker  *brokerImpl
	topic   string
	qos     byte
	handler mqttsvc.MessageHandler
	closed  atomic.Bool
}

func (s *subscriptionImpl) Topic() string { return s.topic }

func (s *subscriptionImpl) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	s.broker.mu.Lock()
	delete(s.broker.subs, s)
	s.broker.topicSubs[s.topic] = removeSub(s.broker.topicSubs[s.topic], s)
	last := len(s.broker.topicSubs[s.topic]) == 0
	if last {
		delete(s.broker.topicSubs, s.topic)
	}
	c := s.broker.client
	connected := s.broker.connected.Load()
	s.broker.mu.Unlock()
	// Only release the Paho subscription when the last interested handler
	// goes away — otherwise sibling subs on the same topic would also stop
	// receiving messages.
	if last && c != nil && connected {
		c.Unsubscribe(s.topic)
	}
	return nil
}

// ── config ────────────────────────────────────────────────────────────────────

type config struct {
	host         string
	port         int
	tls          bool
	tlsInsecure  bool
	caCert       string
	clientID     string
	username     string
	password     string
	keepalive    int
	cleanSession bool
	will         *willConfig
}

type willConfig struct {
	topic   string
	payload string
	qos     byte
	retain  bool
}

func parseConfig(cfg map[string]any) (config, error) {
	c := config{
		port:         1883,
		clientID:     "vdcgo",
		keepalive:    60,
		cleanSession: true,
	}
	c.host, _ = stringField(cfg, "host")
	if c.host == "" {
		return c, errors.New("'host' is required")
	}
	if v, ok := intField(cfg, "port"); ok {
		c.port = v
	}
	if v, ok := boolField(cfg, "tls"); ok {
		c.tls = v
	}
	if v, ok := boolField(cfg, "tlsInsecure"); ok {
		c.tlsInsecure = v
	}
	c.caCert, _ = stringField(cfg, "caCert")
	if v, ok := stringField(cfg, "clientId"); ok && v != "" {
		c.clientID = v
	}
	c.username, _ = stringField(cfg, "username")
	c.password, _ = stringField(cfg, "password")
	if v, ok := intField(cfg, "keepalive"); ok {
		c.keepalive = v
	}
	if v, ok := boolField(cfg, "cleanSession"); ok {
		c.cleanSession = v
	}
	if w, ok := cfg["will"].(map[string]any); ok {
		topic, _ := stringField(w, "topic")
		if topic != "" {
			payload, _ := stringField(w, "payload")
			qos, _ := intField(w, "qos")
			retain, _ := boolField(w, "retain")
			c.will = &willConfig{
				topic:   topic,
				payload: payload,
				qos:     byte(qos),
				retain:  retain,
			}
		}
	}
	if c.tls && c.port == 1883 {
		c.port = 8883
	}
	return c, nil
}

func stringField(m map[string]any, k string) (string, bool) {
	if v, ok := m[k]; ok {
		switch s := v.(type) {
		case string:
			return s, true
		case fmt.Stringer:
			return s.String(), true
		}
	}
	return "", false
}

func intField(m map[string]any, k string) (int, bool) {
	if v, ok := m[k]; ok {
		switch n := v.(type) {
		case int:
			return n, true
		case int64:
			return int(n), true
		case float64:
			return int(n), true
		case string:
			i, err := strconv.Atoi(n)
			if err == nil {
				return i, true
			}
		}
	}
	return 0, false
}

func boolField(m map[string]any, k string) (bool, bool) {
	if v, ok := m[k]; ok {
		switch b := v.(type) {
		case bool:
			return b, true
		case string:
			vb, err := strconv.ParseBool(b)
			if err == nil {
				return vb, true
			}
		}
	}
	return false, false
}

// Probe validates a config without starting a real plugin. It attempts a
// short connect, and disconnects immediately on success.
func Probe(cfg map[string]any) error {
	c, err := parseConfig(cfg)
	if err != nil {
		return err
	}
	scheme := "tcp"
	if c.tls {
		scheme = "tls"
	}
	opts := paho.NewClientOptions().
		AddBroker(fmt.Sprintf("%s://%s:%d", scheme, c.host, c.port)).
		SetClientID(c.clientID + "-probe").
		SetCleanSession(true).
		SetConnectTimeout(5 * time.Second).
		SetAutoReconnect(false).
		SetConnectRetry(false)
	if c.username != "" {
		opts.SetUsername(c.username).SetPassword(c.password)
	}
	if c.tls {
		opts.SetTLSConfig(&tls.Config{InsecureSkipVerify: c.tlsInsecure})
	}
	client := paho.NewClient(opts)
	tok := client.Connect()
	if !tok.WaitTimeout(8 * time.Second) {
		return errors.New("connect timeout")
	}
	if err := tok.Error(); err != nil {
		return err
	}
	client.Disconnect(200)
	return nil
}
