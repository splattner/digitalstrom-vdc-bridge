package bridge

// DeviceDescriptor carries the full initialization descriptor for a vDC device.
// It extends the base Mapping with per-input/sensor/button metadata and
// optional device-level metadata fields.
//
// AnnounceRichDevice uses this to atomically register a device and push all its
// descriptor metadata so that a single ReAnnounce can flush everything to the vDSM.
type DeviceDescriptor struct {
	// Buttons is ordered by button index (0-based). An empty slice means the
	// device uses the default button layout inferred from its output type.
	Buttons []ButtonDescriptor

	// Sensors is ordered by sensor index (0-based). Each entry is pushed via
	// SetSensorDescriptor after the device is announced.
	Sensors []SensorDescriptor

	// Inputs is ordered by binary-input index (0-based). Each entry is pushed via
	// SetBinaryInputDescriptor after the device is announced.
	Inputs []BinaryInputDescriptor
}
