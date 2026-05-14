package protocol

import "fmt"

const (
	MessageInit    = "init"
	MessageInitVDC = "initvdc"
	MessageStatus  = "status"
)

// InitMessage contains a minimal external device declaration.
type InitMessage struct {
	Message  string `json:"message"`
	Protocol string `json:"protocol,omitempty"`
	Tag      string `json:"tag,omitempty"`
	UniqueID string `json:"uniqueid"`
	Output   string `json:"output,omitempty"`
	Name     string `json:"name,omitempty"`
}

// InitVDCMessage allows setting vdc-global metadata.
type InitVDCMessage struct {
	Message       string `json:"message"`
	ModelName     string `json:"modelname,omitempty"`
	ModelVersion  string `json:"modelVersion,omitempty"`
	IconName      string `json:"iconname,omitempty"`
	ConfigURL     string `json:"configurl,omitempty"`
	AlwaysVisible *bool  `json:"alwaysVisible,omitempty"`
	Name          string `json:"name,omitempty"`
}

// StatusMessage is sent as reply to init errors and explicit confirmations.
type StatusMessage struct {
	Message     string `json:"message"`
	Status      string `json:"status"`
	ErrorCode   int    `json:"errorcode,omitempty"`
	ErrorMsg    string `json:"errormessage,omitempty"`
	ErrorDomain string `json:"errordomain,omitempty"`
	Tag         string `json:"tag,omitempty"`
}

func ValidateInit(m InitMessage) error {
	if m.Message != MessageInit {
		return fmt.Errorf("invalid init message type %q", m.Message)
	}
	if m.UniqueID == "" {
		return fmt.Errorf("missing uniqueid")
	}
	if m.Protocol != "" && m.Protocol != "json" && m.Protocol != "simple" {
		return fmt.Errorf("unknown protocol %q", m.Protocol)
	}
	return nil
}
