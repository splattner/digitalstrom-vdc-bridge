package runtime

const (
	EventInit             = "init"
	EventRemove           = "remove"
	EventChannel          = "channel"
	EventButton           = "button"
	EventButtonAction     = "button_action"
	EventInput            = "input"
	EventSensor           = "sensor"
	EventSensorDescriptor = "sensor_descriptor"
	EventActive           = "active"
)

// SensorDescriptor describes the metadata of a single sensor input on a device.
// All fields are optional; the property-tree builder substitutes sensible
// defaults for any zero values.
type SensorDescriptor struct {
	Type       int     // VdcSensorType (sensorType_temperature=1, _humidity=2, …)
	Name       string  // human-readable label (e.g. "temperature")
	Min, Max   float64 // value range in physical units
	Resolution float64 // smallest reportable step
	SIUnit     string  // SI unit code ("celsius", "percent", …)
	Symbol     string  // display symbol ("°C", "%", …)
}

// Event describes relevant runtime updates emitted by external-device connectors.
type Event struct {
	Type             string
	Tag              string
	UniqueID         string
	Name             string
	Output           string
	Index            int
	Value            float64
	Action           string
	Active           bool
	Connection       string
	SensorDescriptor *SensorDescriptor // populated for EventSensorDescriptor
}
