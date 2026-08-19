package vdcapi

import "time"

// PbufTraceFrame is one captured protobuf message, sent to debug subscribers.
type PbufTraceFrame struct {
	// Time is when the frame was captured.
	Time time.Time `json:"time"`
	// Direction is "rx" (received from vDSM) or "tx" (sent to vDSM).
	Direction string `json:"direction"`
	// TypeNum is the raw protobuf message type number.
	TypeNum int `json:"typeNum"`
	// TypeName is the human-readable message type name.
	TypeName string `json:"typeName"`
	// MsgID is the message correlation ID (0 when absent).
	MsgID uint32 `json:"msgId,omitempty"`
	// HasMsgID is true when a message_id was present in the envelope.
	HasMsgID bool `json:"hasMsgId,omitempty"`
	// DeviceDSUID is the primary dSUID extracted from the decoded payload
	// (e.g. the "target" of a getProperty, the first entry in "targets" of a
	// scene notification, the "dsuid" of a hello/ping/remove). Empty when the
	// message is not device-specific (e.g. a getProperty targeting the vDC root).
	DeviceDSUID string `json:"deviceDSUID,omitempty"`
	// Decoded is the decoded message payload as a plain map/slice tree.
	Decoded map[string]any `json:"decoded,omitempty"`
	// Raw is the raw wire bytes (hex-encoded) for low-level inspection.
	RawHex string `json:"rawHex,omitempty"`
}

// ServerConfig holds the fields shared by both the JSON API Server and the
// protobuf PbufServer. Embed this struct in each server type.
type ServerConfig struct {
	Port        int
	DSUID       string
	Description string
	State       *StateStore
	Commander   Commander
	Scenes      *SceneStore
	Config      *ConfigStore
	// OnTrace, when non-nil, is called for every inbound and outbound pbuf frame.
	// It is invoked synchronously; implementations must not block.
	OnTrace func(PbufTraceFrame)
	// RampManager runs the smooth ramps started by dimChannel notifications.
	// Must be a shared instance (e.g. from NewDimRampManager()) across every
	// ServerConfig copy for a given daemon — a new methodService is created
	// per call, so ramp state can only persist here. May be nil, in which
	// case dimChannel notifications are accepted but have no effect.
	RampManager *dimRampManager
}

func (c ServerConfig) methodService() methodService {
	return newMethodService(c.DSUID, c.Description, c.State, c.Commander, c.Scenes, c.Config, c.RampManager)
}
