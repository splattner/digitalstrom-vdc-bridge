package runtime

const (
	EventInit                  = "init"
	EventRemove                = "remove"
	EventChannel               = "channel"
	EventButton                = "button"
	EventButtonAction          = "button_action"
	EventInput                 = "input"
	EventSensor                = "sensor"
	EventSensorDescriptor      = "sensor_descriptor"
	EventBinaryInputDescriptor = "binary_input_descriptor"
	EventActive                = "active"
)

// SensorDescriptor describes the metadata of a single sensor input on a device.
// All fields are optional; the property-tree builder substitutes sensible
// defaults for any zero values.
type SensorDescriptor struct {
	Type       int     // VdcSensorType (sensorType_temperature=1, _humidity=2, …)
	Usage      int     // VdcUsageHint (0=undefined, 1=room, 2=outdoors, 3=user). Required for
	//                 dSS to map (type,usage) to a concrete dS sensorInput.type — e.g.
	//                 (temperature, room) → dSS type 9; with usage=0 it stays type 255.
	Name       string  // human-readable label (e.g. "temperature")
	Min, Max   float64 // value range in physical units
	Resolution float64 // smallest reportable step
	SIUnit     string  // SI unit code ("celsius", "percent", …)
	Symbol     string  // display symbol ("°C", "%", …)
}

// BinaryInputDescriptor describes the metadata of a single binary input on a device.
// The Function field uses the dS binaryInput sensorFunction enum
// (0=none, 1=presence, 5=motion, 7=smoke, 13=window_closed, 14=door_closed, …).
type BinaryInputDescriptor struct {
	Name     string
	Function int
}

// Event describes relevant runtime updates emitted by external-device connectors.
type Event struct {
	Type                  string
	Tag                   string
	UniqueID              string
	Name                  string
	Output                string
	Index                 int
	Value                 float64
	Action                string
	Active                bool
	Connection            string
	SensorDescriptor      *SensorDescriptor      // populated for EventSensorDescriptor
	BinaryInputDescriptor *BinaryInputDescriptor // populated for EventBinaryInputDescriptor
}
