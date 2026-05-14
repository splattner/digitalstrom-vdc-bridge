package vdcapi

type sceneActionKind int

const (
	sceneActionIgnore sceneActionKind = iota
	sceneActionSetLevel
	sceneActionDimUp
	sceneActionDimDown
	sceneActionStop
)

type sceneAction struct {
	kind  sceneActionKind
	level float64
}

func mapSceneAction(scene int) sceneAction {
	if level, ok := sceneBrightnessLevels[scene]; ok {
		return sceneAction{kind: sceneActionSetLevel, level: level}
	}
	switch scene {
	case 11, 42, 44, 46, 48:
		return sceneAction{kind: sceneActionDimDown}
	case 12, 43, 45, 47, 49:
		return sceneAction{kind: sceneActionDimUp}
	case 15, 52, 53, 54, 55:
		return sceneAction{kind: sceneActionStop}
	default:
		return sceneAction{kind: sceneActionIgnore}
	}
}

var sceneBrightnessLevels = map[int]float64{
	0: 0, 1: 0, 2: 0, 3: 0, 4: 0,
	5: 100, 6: 100, 7: 100, 8: 100, 9: 100,
	13: 1, 14: 100,
	17: 75, 18: 50, 19: 25,
	20: 75, 21: 50, 22: 25,
	23: 75, 24: 65, 25: 64,
	26: 75, 27: 65, 28: 25,
	29: 75, 30: 65, 31: 25,
	32: 0, 33: 100, 34: 0, 35: 100, 36: 0, 37: 100, 38: 0, 39: 100,
	40: 0,
	50: 0, 51: 100,
	64: 0, 65: 100, 67: 0, 68: 0, 69: 0, 70: 100, 71: 100, 72: 0,
	74: 100, 75: 100, 76: 100,
}
