package server

import (
	"encoding/json"
	"errors"

	"github.com/splattner/vdcgo/pkg/protocol"
)

type statusError struct {
	tag string
	err error
}

func (e statusError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func withStatusTag(tag string, err error) error {
	if err == nil {
		return nil
	}
	return statusError{tag: tag, err: err}
}

func splitStatusError(err error) (string, error) {
	var se statusError
	if errors.As(err, &se) {
		return se.tag, se.err
	}
	return "", err
}

func (c *Connector) sendStatus(err error, tag string) error {
	if c.mode == modeSimple {
		line := protocol.FormatSimpleStatus(err == nil, errorText(err), tag)
		return c.writeLine(line)
	}
	msg := protocol.StatusMessage{
		Message: protocol.MessageStatus,
		Status:  "ok",
		Tag:     tag,
	}
	if err != nil {
		msg.Status = "error"
		msg.ErrorCode = 1
		msg.ErrorMsg = errorText(err)
		msg.ErrorDomain = "vdcgo"
	}
	b, jerr := json.Marshal(msg)
	if jerr != nil {
		return jerr
	}
	return c.writeLine(string(b))
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
