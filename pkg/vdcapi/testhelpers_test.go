package vdcapi

// mockCommander implements Commander for tests.
type mockCommander struct {
	called       bool
	uniqueID     string
	value        float64
	err          error
	colorCalled  bool
	colorUID     string
	colorChannel int
	colorValue   float64
}

func (m *mockCommander) SetLightLevel(uniqueID string, value float64) error {
	m.called = true
	m.uniqueID = uniqueID
	m.value = value
	return m.err
}

func (m *mockCommander) SetChannelValue(uniqueID string, channelIndex int, value float64) error {
	if channelIndex == 0 {
		m.called = true
		m.uniqueID = uniqueID
		m.value = value
	} else {
		m.colorCalled = true
		m.colorUID = uniqueID
		m.colorChannel = channelIndex
		m.colorValue = value
	}
	return m.err
}
