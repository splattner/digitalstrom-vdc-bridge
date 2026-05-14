package protocol

import (
	"fmt"
	"strings"
)

// SimpleMessage represents one line from simple runtime protocol.
type SimpleMessage struct {
	Tag   string
	Cmd   string
	Value string
}

func ParseSimpleLine(line string) (SimpleMessage, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return SimpleMessage{}, fmt.Errorf("empty message")
	}

	msg := SimpleMessage{}
	body := line
	if i := strings.IndexByte(line, ':'); i >= 0 {
		msg.Tag = strings.TrimSpace(line[:i])
		body = strings.TrimSpace(line[i+1:])
	}

	if i := strings.IndexByte(body, '='); i >= 0 {
		msg.Cmd = strings.TrimSpace(body[:i])
		msg.Value = strings.TrimSpace(body[i+1:])
	} else {
		msg.Cmd = strings.TrimSpace(body)
	}
	if msg.Cmd == "" {
		return SimpleMessage{}, fmt.Errorf("missing command")
	}
	return msg, nil
}

func FormatSimpleStatus(ok bool, errMsg string, tag string) string {
	body := "OK"
	if !ok {
		body = "ERROR=" + errMsg
	}
	if tag != "" {
		return tag + ":" + body
	}
	return body
}
