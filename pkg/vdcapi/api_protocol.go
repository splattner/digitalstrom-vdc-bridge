package vdcapi

// APIVersionMin and APIVersionMax define the range of vDC API versions
// this server accepts during the hello handshake.
const (
	APIVersionMin = 2
	APIVersionMax = 3
)

// session tracks the state of a connected vDSM client connection.
type session struct {
	active     bool
	vdsmDSUID  string
	apiVersion int
}

// request is the parsed form of an inbound vDC API message in the internal
// representation (populated from either JSON or protobuf wire framing).
type request struct {
	ID           any
	Method       string
	Notification string
	Params       map[string]any
	Raw          map[string]any
}

// response is the outbound vDC API response in the internal representation.
type response struct {
	ID          any    `json:"id,omitempty"`
	Result      any    `json:"result,omitempty"`
	Error       int    `json:"error,omitempty"`
	ErrorMsg    string `json:"errormessage,omitempty"`
	ErrorDomain string `json:"errordomain,omitempty"`
}

// Server is the vDC API request dispatcher.  It processes vDC API requests in
// the internal request/response representation and contains all method and
// notification dispatch logic shared across wire-protocol implementations.
type Server struct {
	ServerConfig
}
