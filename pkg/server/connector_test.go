package server

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/splattner/vdcgo/pkg/runtime"
)

func waitFor(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func TestConnectorInitJSONStatus(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := NewConnector(server, nil)
	go c.Run(ctx)

	_, err := client.Write([]byte(`{"message":"init","uniqueid":"u1","output":"light"}` + "\n"))
	if err != nil {
		t.Fatalf("write init: %v", err)
	}

	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	line, err := bufio.NewReader(client).ReadString('\n')
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(line, `"status":"ok"`) {
		t.Fatalf("expected ok status, got: %s", line)
	}
}

func TestConnectorInitSimpleSwitchAndStatus(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := NewConnector(server, nil)
	go c.Run(ctx)

	_, err := client.Write([]byte(`{"message":"init","protocol":"simple","uniqueid":"u2","tag":"A"}` + "\n"))
	if err != nil {
		t.Fatalf("write init: %v", err)
	}

	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	line, err := bufio.NewReader(client).ReadString('\n')
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if strings.TrimSpace(line) != "A:OK" {
		t.Fatalf("expected simple OK status, got: %s", line)
	}
}

func TestConnectorJSONRuntimeValidationError(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := NewConnector(server, nil)
	go c.Run(ctx)

	_, err := client.Write([]byte(`{"message":"init","uniqueid":"u3"}` + "\n"))
	if err != nil {
		t.Fatalf("write init: %v", err)
	}

	r := bufio.NewReader(client)
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatalf("read init status: %v", err)
	}

	_, err = client.Write([]byte(`{"message":"channel","index":0}` + "\n"))
	if err != nil {
		t.Fatalf("write invalid runtime message: %v", err)
	}

	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read runtime error status: %v", err)
	}
	if !strings.Contains(line, `"status":"error"`) || !strings.Contains(line, "missing 'value'") {
		t.Fatalf("expected validation error status, got: %s", line)
	}
}

func TestConnectorSimpleTaggedError(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := NewConnector(server, nil)
	go c.Run(ctx)

	r := bufio.NewReader(client)
	_, err := client.Write([]byte(`{"message":"init","protocol":"simple","tag":"A","uniqueid":"u4"}` + "\n"))
	if err != nil {
		t.Fatalf("write init: %v", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatalf("read init status: %v", err)
	}

	_, err = client.Write([]byte("A:XYZ=1\n"))
	if err != nil {
		t.Fatalf("write invalid simple message: %v", err)
	}

	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read simple error status: %v", err)
	}
	if strings.TrimSpace(line) != `A:ERROR=unknown message "XYZ"` {
		t.Fatalf("unexpected simple tagged error: %s", line)
	}
}

func TestConnectorJSONChannelUpdatesState(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := NewConnector(server, nil)
	go c.Run(ctx)

	r := bufio.NewReader(client)
	_, err := client.Write([]byte(`{"message":"init","uniqueid":"u5"}` + "\n"))
	if err != nil {
		t.Fatalf("write init: %v", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatalf("read init status: %v", err)
	}

	_, err = client.Write([]byte(`{"message":"channel","index":0,"value":42.5}` + "\n"))
	if err != nil {
		t.Fatalf("write channel: %v", err)
	}

	waitFor(t, 500*time.Millisecond, func() bool {
		state, ok := c.registry.Snapshot("")
		if !ok {
			return false
		}
		return state.Channels[0] == 42.5
	})
}

func TestConnectorSimpleChannelUpdatesState(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := NewConnector(server, nil)
	go c.Run(ctx)

	r := bufio.NewReader(client)
	_, err := client.Write([]byte(`{"message":"init","protocol":"simple","tag":"A","uniqueid":"u6"}` + "\n"))
	if err != nil {
		t.Fatalf("write init: %v", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatalf("read init status: %v", err)
	}

	_, err = client.Write([]byte("A:C0=12.25\n"))
	if err != nil {
		t.Fatalf("write simple channel: %v", err)
	}

	waitFor(t, 500*time.Millisecond, func() bool {
		state, ok := c.registry.Snapshot("A")
		if !ok {
			return false
		}
		return state.Channels[0] == 12.25
	})
}

func TestConnectorSendLightLevelSimple(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := NewConnector(server, nil)
	go c.Run(ctx)

	r := bufio.NewReader(client)
	_, err := client.Write([]byte(`{"message":"init","protocol":"simple","tag":"A","uniqueid":"u7"}` + "\n"))
	if err != nil {
		t.Fatalf("write init: %v", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatalf("read init status: %v", err)
	}

	lineCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
		line, err := r.ReadString('\n')
		if err != nil {
			errCh <- err
			return
		}
		lineCh <- line
	}()

	ok, err := c.SendLightLevel("u7", 33.3)
	if err != nil {
		t.Fatalf("send light level: %v", err)
	}
	if !ok {
		t.Fatal("expected command to be delivered")
	}

	var line string
	select {
	case line = <-lineCh:
	case err := <-errCh:
		t.Fatalf("read command line: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for command line")
	}
	if strings.TrimSpace(line) != "A:C0=33.300000" {
		t.Fatalf("unexpected outbound command: %s", line)
	}
}

func TestConnectorEmitsTypedInputEvents(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	events := make(chan runtime.Event, 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := NewConnector(server, func(e runtime.Event) {
		events <- e
	})
	go c.Run(ctx)

	r := bufio.NewReader(client)
	_, err := client.Write([]byte(`{"message":"init","uniqueid":"u8"}` + "\n"))
	if err != nil {
		t.Fatalf("write init: %v", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatalf("read init status: %v", err)
	}

	_, err = client.Write([]byte(`{"message":"button","index":1,"value":1}` + "\n"))
	if err != nil {
		t.Fatalf("write button: %v", err)
	}
	_, err = client.Write([]byte(`{"message":"input","index":0,"value":0}` + "\n"))
	if err != nil {
		t.Fatalf("write input: %v", err)
	}
	_, err = client.Write([]byte(`{"message":"sensor","index":2,"value":24.5}` + "\n"))
	if err != nil {
		t.Fatalf("write sensor: %v", err)
	}

	seen := map[string]bool{}
	deadline := time.After(2 * time.Second)
	for len(seen) < 3 {
		select {
		case e := <-events:
			switch e.Type {
			case runtime.EventButton:
				if e.Index == 1 && e.Value == 1 {
					seen[e.Type] = true
				}
			case runtime.EventInput:
				if e.Index == 0 && e.Value == 0 {
					seen[e.Type] = true
				}
			case runtime.EventSensor:
				if e.Index == 2 && e.Value == 24.5 {
					seen[e.Type] = true
				}
			}
		case <-deadline:
			t.Fatalf("did not receive all typed events, seen=%+v", seen)
		}
	}
}

func TestConnectorEmitsButtonTipAction(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	events := make(chan runtime.Event, 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := NewConnector(server, func(e runtime.Event) {
		events <- e
	})
	go c.Run(ctx)

	r := bufio.NewReader(client)
	_, err := client.Write([]byte(`{"message":"init","uniqueid":"u9"}` + "\n"))
	if err != nil {
		t.Fatalf("write init: %v", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatalf("read init status: %v", err)
	}

	_, err = client.Write([]byte(`{"message":"button","index":0,"value":1}` + "\n"))
	if err != nil {
		t.Fatalf("write button press: %v", err)
	}
	_, err = client.Write([]byte(`{"message":"button","index":0,"value":0}` + "\n"))
	if err != nil {
		t.Fatalf("write button release: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case e := <-events:
			if e.Type == runtime.EventButtonAction && e.Index == 0 && e.Action == "tip" {
				return
			}
		case <-deadline:
			t.Fatal("did not receive button tip action event")
		}
	}
}

func TestConnectorEmitsButtonMultiClickTip2(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	events := make(chan runtime.Event, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := NewConnector(server, func(e runtime.Event) {
		events <- e
	})
	go c.Run(ctx)

	r := bufio.NewReader(client)
	_, err := client.Write([]byte(`{"message":"init","uniqueid":"u10"}` + "\n"))
	if err != nil {
		t.Fatalf("write init: %v", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatalf("read init status: %v", err)
	}

	for i := 0; i < 2; i++ {
		_, err = client.Write([]byte(`{"message":"button","index":0,"value":1}` + "\n"))
		if err != nil {
			t.Fatalf("write button press: %v", err)
		}
		_, err = client.Write([]byte(`{"message":"button","index":0,"value":0}` + "\n"))
		if err != nil {
			t.Fatalf("write button release: %v", err)
		}
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case e := <-events:
			if e.Type == runtime.EventButtonAction && e.Index == 0 && e.Action == "tip2" {
				return
			}
		case <-deadline:
			t.Fatal("did not receive button tip2 action event")
		}
	}
}

func TestConnectorButtonMultiClickResetsAfterTimeout(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	events := make(chan runtime.Event, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := NewConnector(server, func(e runtime.Event) {
		events <- e
	})
	go c.Run(ctx)

	r := bufio.NewReader(client)
	_, err := client.Write([]byte(`{"message":"init","uniqueid":"u11"}` + "\n"))
	if err != nil {
		t.Fatalf("write init: %v", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatalf("read init status: %v", err)
	}

	_, err = client.Write([]byte(`{"message":"button","index":0,"value":1}` + "\n"))
	if err != nil {
		t.Fatalf("write first press: %v", err)
	}
	_, err = client.Write([]byte(`{"message":"button","index":0,"value":0}` + "\n"))
	if err != nil {
		t.Fatalf("write first release: %v", err)
	}

	time.Sleep(buttonMultiClickWindow + 100*time.Millisecond)

	_, err = client.Write([]byte(`{"message":"button","index":0,"value":1}` + "\n"))
	if err != nil {
		t.Fatalf("write second press: %v", err)
	}
	_, err = client.Write([]byte(`{"message":"button","index":0,"value":0}` + "\n"))
	if err != nil {
		t.Fatalf("write second release: %v", err)
	}

	tipCount := 0
	deadline := time.After(3 * time.Second)
	for {
		select {
		case e := <-events:
			if e.Type == runtime.EventButtonAction && e.Index == 0 && e.Action == "tip" {
				tipCount++
				if tipCount >= 2 {
					return
				}
			}
			if e.Type == runtime.EventButtonAction && e.Index == 0 && e.Action == "tip2" {
				t.Fatalf("expected reset to tip after timeout, got tip2")
			}
		case <-deadline:
			t.Fatalf("did not observe expected reset tips, count=%d", tipCount)
		}
	}
}

func TestConnectorEmitsHoldWithDimRepeat(t *testing.T) {
	prevHold := buttonHoldThreshold
	prevRepeat := buttonDimRepeatInterval
	prevDebounce := buttonDebounceWindow
	buttonHoldThreshold = 80 * time.Millisecond
	buttonDimRepeatInterval = 40 * time.Millisecond
	buttonDebounceWindow = 5 * time.Millisecond
	defer func() {
		buttonHoldThreshold = prevHold
		buttonDimRepeatInterval = prevRepeat
		buttonDebounceWindow = prevDebounce
	}()

	client, server := net.Pipe()
	defer client.Close()

	events := make(chan runtime.Event, 32)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := NewConnector(server, func(e runtime.Event) {
		events <- e
	})
	go c.Run(ctx)

	r := bufio.NewReader(client)
	_, err := client.Write([]byte(`{"message":"init","uniqueid":"u12"}` + "\n"))
	if err != nil {
		t.Fatalf("write init: %v", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatalf("read init status: %v", err)
	}

	_, err = client.Write([]byte(`{"message":"button","index":0,"value":1}` + "\n"))
	if err != nil {
		t.Fatalf("write button press: %v", err)
	}
	time.Sleep(190 * time.Millisecond)
	_, err = client.Write([]byte(`{"message":"button","index":0,"value":0}` + "\n"))
	if err != nil {
		t.Fatalf("write button release: %v", err)
	}

	holdSeen := false
	dimUpCount := 0
	deadline := time.After(2 * time.Second)
	for {
		if holdSeen && dimUpCount >= 2 {
			return
		}
		select {
		case e := <-events:
			if e.Type != runtime.EventButtonAction || e.Index != 0 {
				continue
			}
			if e.Action == "hold" {
				holdSeen = true
			}
			if e.Action == "dimup" {
				dimUpCount++
			}
		case <-deadline:
			t.Fatalf("expected hold + dimup repeat actions, got hold=%t dimupCount=%d", holdSeen, dimUpCount)
		}
	}
}

func TestConnectorDebounceSuppressesShortTapAction(t *testing.T) {
	prevHold := buttonHoldThreshold
	prevDebounce := buttonDebounceWindow
	buttonHoldThreshold = 200 * time.Millisecond
	buttonDebounceWindow = 80 * time.Millisecond
	defer func() {
		buttonHoldThreshold = prevHold
		buttonDebounceWindow = prevDebounce
	}()

	client, server := net.Pipe()
	defer client.Close()

	events := make(chan runtime.Event, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := NewConnector(server, func(e runtime.Event) {
		events <- e
	})
	go c.Run(ctx)

	r := bufio.NewReader(client)
	_, err := client.Write([]byte(`{"message":"init","uniqueid":"u13"}` + "\n"))
	if err != nil {
		t.Fatalf("write init: %v", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatalf("read init status: %v", err)
	}

	_, err = client.Write([]byte(`{"message":"button","index":0,"value":1}` + "\n"))
	if err != nil {
		t.Fatalf("write button press: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	_, err = client.Write([]byte(`{"message":"button","index":0,"value":0}` + "\n"))
	if err != nil {
		t.Fatalf("write button release: %v", err)
	}

	deadline := time.After(250 * time.Millisecond)
	for {
		select {
		case e := <-events:
			if e.Type == runtime.EventButtonAction && e.Index == 0 {
				t.Fatalf("did not expect button action from debounced short tap, got %s", e.Action)
			}
		case <-deadline:
			return
		}
	}
}

func TestConnectorButtonModeSingleDisablesMultiClick(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	events := make(chan runtime.Event, 32)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := NewConnector(server, func(e runtime.Event) {
		events <- e
	})
	go c.Run(ctx)

	r := bufio.NewReader(client)
	_, err := client.Write([]byte(`{"message":"init","uniqueid":"u14"}` + "\n"))
	if err != nil {
		t.Fatalf("write init: %v", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatalf("read init status: %v", err)
	}

	for i := 0; i < 2; i++ {
		_, err = client.Write([]byte(`{"message":"button","index":0,"value":1,"mode":"single"}` + "\n"))
		if err != nil {
			t.Fatalf("write button press: %v", err)
		}
		_, err = client.Write([]byte(`{"message":"button","index":0,"value":0,"mode":"single"}` + "\n"))
		if err != nil {
			t.Fatalf("write button release: %v", err)
		}
	}

	tipCount := 0
	deadline := time.After(500 * time.Millisecond)
	for {
		select {
		case e := <-events:
			if e.Type != runtime.EventButtonAction || e.Index != 0 {
				continue
			}
			if e.Action == "tip2" || e.Action == "tip3" || e.Action == "tip4" {
				t.Fatalf("did not expect multi-click action in single mode, got %s", e.Action)
			}
			if e.Action == "tip" {
				tipCount++
			}
		case <-deadline:
			if tipCount < 2 {
				t.Fatalf("expected at least two tip actions in single mode, got %d", tipCount)
			}
			return
		}
	}
}

func TestConnectorButtonModeDimmerSuppressesHoldAction(t *testing.T) {
	prevHold := buttonHoldThreshold
	prevRepeat := buttonDimRepeatInterval
	buttonHoldThreshold = 80 * time.Millisecond
	buttonDimRepeatInterval = 40 * time.Millisecond
	defer func() {
		buttonHoldThreshold = prevHold
		buttonDimRepeatInterval = prevRepeat
	}()

	client, server := net.Pipe()
	defer client.Close()

	events := make(chan runtime.Event, 32)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := NewConnector(server, func(e runtime.Event) {
		events <- e
	})
	go c.Run(ctx)

	r := bufio.NewReader(client)
	_, err := client.Write([]byte(`{"message":"init","uniqueid":"u15"}` + "\n"))
	if err != nil {
		t.Fatalf("write init: %v", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatalf("read init status: %v", err)
	}

	_, err = client.Write([]byte(`{"message":"button","index":1,"value":1,"mode":"dimmer"}` + "\n"))
	if err != nil {
		t.Fatalf("write button press: %v", err)
	}
	time.Sleep(190 * time.Millisecond)
	_, err = client.Write([]byte(`{"message":"button","index":1,"value":0,"mode":"dimmer"}` + "\n"))
	if err != nil {
		t.Fatalf("write button release: %v", err)
	}

	holdSeen := false
	dimDownCount := 0
	deadline := time.After(600 * time.Millisecond)
	for {
		select {
		case e := <-events:
			if e.Type != runtime.EventButtonAction || e.Index != 1 {
				continue
			}
			if e.Action == "hold" {
				holdSeen = true
			}
			if e.Action == "dimdown" {
				dimDownCount++
			}
		case <-deadline:
			if holdSeen {
				t.Fatal("did not expect hold action in dimmer mode")
			}
			if dimDownCount == 0 {
				t.Fatal("expected dimdown repeat action in dimmer mode")
			}
			return
		}
	}
}

func TestConnectorButtonModeSceneMapsClickActions(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	events := make(chan runtime.Event, 32)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := NewConnector(server, func(e runtime.Event) {
		events <- e
	})
	go c.Run(ctx)

	r := bufio.NewReader(client)
	_, err := client.Write([]byte(`{"message":"init","uniqueid":"u16"}` + "\n"))
	if err != nil {
		t.Fatalf("write init: %v", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatalf("read init status: %v", err)
	}

	for i := 0; i < 2; i++ {
		_, err = client.Write([]byte(`{"message":"button","index":0,"value":1,"mode":"scene"}` + "\n"))
		if err != nil {
			t.Fatalf("write button press: %v", err)
		}
		_, err = client.Write([]byte(`{"message":"button","index":0,"value":0,"mode":"scene"}` + "\n"))
		if err != nil {
			t.Fatalf("write button release: %v", err)
		}
	}

	seenScene5 := false
	seenScene0 := false
	deadline := time.After(600 * time.Millisecond)
	for {
		if seenScene5 && seenScene0 {
			return
		}
		select {
		case e := <-events:
			if e.Type != runtime.EventButtonAction || e.Index != 0 {
				continue
			}
			if e.Action == "tip" || e.Action == "tip2" {
				t.Fatalf("did not expect raw tip action in scene mode, got %s", e.Action)
			}
			if e.Action == "scene5" {
				seenScene5 = true
			}
			if e.Action == "scene0" {
				seenScene0 = true
			}
		case <-deadline:
			t.Fatalf("expected scene-mode mapped actions scene5+scene0, got scene5=%t scene0=%t", seenScene5, seenScene0)
		}
	}
}

func TestConnectorButtonModeSceneMapsHoldToScene(t *testing.T) {
	prevHold := buttonHoldThreshold
	prevRepeat := buttonDimRepeatInterval
	buttonHoldThreshold = 80 * time.Millisecond
	buttonDimRepeatInterval = 40 * time.Millisecond
	defer func() {
		buttonHoldThreshold = prevHold
		buttonDimRepeatInterval = prevRepeat
	}()

	client, server := net.Pipe()
	defer client.Close()

	events := make(chan runtime.Event, 32)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := NewConnector(server, func(e runtime.Event) {
		events <- e
	})
	go c.Run(ctx)

	r := bufio.NewReader(client)
	_, err := client.Write([]byte(`{"message":"init","uniqueid":"u17"}` + "\n"))
	if err != nil {
		t.Fatalf("write init: %v", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatalf("read init status: %v", err)
	}

	_, err = client.Write([]byte(`{"message":"button","index":1,"value":1,"mode":"scene"}` + "\n"))
	if err != nil {
		t.Fatalf("write button press: %v", err)
	}
	time.Sleep(190 * time.Millisecond)
	_, err = client.Write([]byte(`{"message":"button","index":1,"value":0,"mode":"scene"}` + "\n"))
	if err != nil {
		t.Fatalf("write button release: %v", err)
	}

	seenScene42 := false
	deadline := time.After(600 * time.Millisecond)
	for {
		select {
		case e := <-events:
			if e.Type != runtime.EventButtonAction || e.Index != 1 {
				continue
			}
			if e.Action == "hold" || e.Action == "dimdown" {
				t.Fatalf("did not expect raw hold/dim action in scene mode, got %s", e.Action)
			}
			if e.Action == "scene42" {
				seenScene42 = true
			}
		case <-deadline:
			if !seenScene42 {
				t.Fatal("expected scene42 mapped hold action in scene mode")
			}
			return
		}
	}
}
