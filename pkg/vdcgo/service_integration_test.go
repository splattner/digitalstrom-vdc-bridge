package vdcgo

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/splattner/vdcgo/pkg/bridge"
	"github.com/splattner/vdcgo/pkg/bridge/externaldevice"
)

func TestServiceIntegrationProtobufHarness(t *testing.T) {
	vdcPort := reserveTCPPort(t)
	sockPath := filepath.Join(t.TempDir(), "vdcgo-ext.sock")

	svc, err := NewService(Config{
		VdcAPIPort:   vdcPort,
		EnableVdcAPI: true,
		EnableDNSSD:  false,
		Description:  "integration-pbuf",
		PluginConfigs: []bridge.PluginConfig{
			{
				ID:   "ext-test",
				Type: externaldevice.PluginType,
				Config: map[string]any{
					"listen":   sockPath,
					"nonlocal": false,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(ctx)
	}()
	defer func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("service exited with error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("service did not stop in time")
		}
	}()

	// waitDial implicitly waits for the externaldevice plugin to start (socket appears).
	ext := waitDial(t, "unix", sockPath, 3*time.Second)
	defer ext.Close()
	rExt := bufio.NewReader(ext)
	_, err = ext.Write([]byte(`{"message":"init","uniqueid":"u-it-pbuf","output":"light","name":"itest-pbuf"}` + "\n"))
	if err != nil {
		t.Fatalf("write init: %v", err)
	}
	_ = ext.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := rExt.ReadString('\n'); err != nil {
		t.Fatalf("read init status: %v", err)
	}
	// The server sends the ack only after emitting EventInit, so the device is now
	// registered in the plugin. Create the bridge mapping before the vDSM connects so
	// both AnnounceDevice calls (from CreateBridge + Subscribe) land in the state store
	// before the vDSM subscribes — the vDSM then sees a clean single-device snapshot.
	if _, err := svc.bridges.CreateBridge(ctx, "ext-test", "u-it-pbuf", "itest-pbuf", "light"); err != nil {
		t.Fatalf("CreateBridge: %v", err)
	}

	apiConn := waitDial(t, "tcp", fmt.Sprintf("127.0.0.1:%d", vdcPort), 3*time.Second)
	defer apiConn.Close()

	helloReq := make([]byte, 0, 64)
	helloReq = protowire.AppendTag(helloReq, 1, protowire.VarintType)
	helloReq = protowire.AppendVarint(helloReq, 2)
	helloReq = protowire.AppendTag(helloReq, 2, protowire.VarintType)
	helloReq = protowire.AppendVarint(helloReq, 7)
	helloBody := make([]byte, 0, 32)
	helloBody = protowire.AppendTag(helloBody, 1, protowire.BytesType)
	helloBody = protowire.AppendString(helloBody, "001122")
	helloBody = protowire.AppendTag(helloBody, 2, protowire.VarintType)
	helloBody = protowire.AppendVarint(helloBody, 2)
	helloReq = protowire.AppendTag(helloReq, 100, protowire.BytesType)
	helloReq = protowire.AppendBytes(helloReq, helloBody)
	if err := writePbufFrame(apiConn, helloReq); err != nil {
		t.Fatalf("write hello frame: %v", err)
	}

	types := make(map[uint64]bool)
	for i := 0; i < 5; i++ {
		_ = apiConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		payload, err := readPbufFrame(apiConn)
		if err != nil {
			t.Fatalf("read hello response frame %d: %v", i, err)
		}
		types[firstFieldVarint(payload, 1)] = true
	}
	// Expect: helloResponse(3) + announceVdc(23) + vanish(11) + announceDevice(10) + pushNotification(12)
	if !types[3] || !types[23] || !types[11] || !types[10] || !types[12] {
		t.Fatalf("expected hello+announce+vanish+push frame types 3/23/11/10/12, got %+v", types)
	}

	getReq := make([]byte, 0, 80)
	getReq = protowire.AppendTag(getReq, 1, protowire.VarintType)
	getReq = protowire.AppendVarint(getReq, 4)
	getReq = protowire.AppendTag(getReq, 2, protowire.VarintType)
	getReq = protowire.AppendVarint(getReq, 8)
	getBody := make([]byte, 0, 16)
	getBody = protowire.AppendTag(getBody, 1, protowire.BytesType)
	getBody = protowire.AppendString(getBody, "root")
	getReq = protowire.AppendTag(getReq, 102, protowire.BytesType)
	getReq = protowire.AppendBytes(getReq, getBody)
	if err := writePbufFrame(apiConn, getReq); err != nil {
		t.Fatalf("write getProperty frame: %v", err)
	}
	_ = apiConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	getResp, err := readPbufFrame(apiConn)
	if err != nil {
		t.Fatalf("read getProperty response: %v", err)
	}
	if gotType := firstFieldVarint(getResp, 1); gotType != 5 {
		t.Fatalf("expected getProperty response type=5, got %d", gotType)
	}

	_, err = ext.Write([]byte(`{"message":"channel","index":0,"value":61}` + "\n"))
	if err != nil {
		t.Fatalf("write channel update: %v", err)
	}
	_ = apiConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	push, err := readPbufFrame(apiConn)
	if err != nil {
		t.Fatalf("read push frame: %v", err)
	}
	if gotType := firstFieldVarint(push, 1); gotType != 12 {
		t.Fatalf("expected push notification type=12, got %d", gotType)
	}
}

func reserveTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve tcp port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func waitDial(t *testing.T, network, addr string, timeout time.Duration) net.Conn {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout(network, addr, 200*time.Millisecond)
		if err == nil {
			return conn
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timeout dialing %s %s", network, addr)
	return nil
}

func writePbufFrame(conn net.Conn, payload []byte) error {
	frame := make([]byte, 2+len(payload))
	binary.BigEndian.PutUint16(frame[:2], uint16(len(payload)))
	copy(frame[2:], payload)
	_, err := conn.Write(frame)
	return err
}

func readPbufFrame(conn net.Conn) ([]byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, err
	}
	n := int(binary.BigEndian.Uint16(header))
	payload := make([]byte, n)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func firstFieldVarint(payload []byte, fieldNum protowire.Number) uint64 {
	for len(payload) > 0 {
		num, typ, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return 0
		}
		payload = payload[n:]
		if num == fieldNum && typ == protowire.VarintType {
			v, m := protowire.ConsumeVarint(payload)
			if m < 0 {
				return 0
			}
			return v
		}
		m := protowire.ConsumeFieldValue(num, typ, payload)
		if m < 0 {
			return 0
		}
		payload = payload[m:]
	}
	return 0
}
