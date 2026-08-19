package server

import (
	"strconv"
	"time"

	"github.com/splattner/vdcgo/pkg/runtime"
)

var buttonHoldThreshold = 700 * time.Millisecond
var buttonMultiClickWindow = 450 * time.Millisecond
var buttonDimRepeatInterval = 350 * time.Millisecond
var buttonDebounceWindow time.Duration

type buttonSequenceState struct {
	pressedAt   time.Time
	lastRelease time.Time
	clickCount  int
	mode        string
}

func (c *Connector) emitButtonAction(tag, uniqueID string, index int, value float64, mode string) {
	key := tag + "#" + strconv.Itoa(index)
	now := time.Now()
	state := c.buttonStates[key]
	if m := normalizeButtonMode(mode); m != "" {
		state.mode = m
	}
	pressed := value > 0
	if pressed {
		if state.pressedAt.IsZero() {
			state.pressedAt = now
			c.buttonStates[key] = state
		}
		return
	}
	if state.pressedAt.IsZero() {
		return
	}
	pressedAt := state.pressedAt
	state.pressedAt = time.Time{}
	pressDuration := now.Sub(pressedAt)
	if pressDuration < buttonDebounceWindow {
		c.buttonStates[key] = state
		return
	}
	// Both branches below assign action before it is read.
	var action string
	if pressDuration >= buttonHoldThreshold {
		state.clickCount = 0
		action = "hold"
		c.buttonStates[key] = state
		mappedHold := mapButtonActionByMode(state.mode, index, action)
		if state.mode != "dimmer" {
			c.emitEvent(runtime.Event{Type: runtime.EventButtonAction, Tag: tag, UniqueID: uniqueID, Index: index, Action: mappedHold})
		}
		repeatCount := buttonRepeatCount(pressDuration)
		repeatAction := buttonDimRepeatAction(index)
		if state.mode == "dimmer" || state.mode == "default" || state.mode == "" {
			for i := 0; i < repeatCount; i++ {
				mappedRepeat := mapButtonActionByMode(state.mode, index, repeatAction)
				c.emitEvent(runtime.Event{Type: runtime.EventButtonAction, Tag: tag, UniqueID: uniqueID, Index: index, Action: mappedRepeat})
			}
		}
		return
	}
	multiClickEnabled := state.mode != "single" && state.mode != "dimmer"
	if !multiClickEnabled {
		state.clickCount = 1
	} else if state.lastRelease.IsZero() || now.Sub(state.lastRelease) > buttonMultiClickWindow {
		state.clickCount = 1
	} else {
		state.clickCount++
		if state.clickCount > 4 {
			state.clickCount = 4
		}
	}
	state.lastRelease = now
	action = buttonClickAction(state.clickCount)
	mappedAction := mapButtonActionByMode(state.mode, index, action)
	c.buttonStates[key] = state
	c.emitEvent(runtime.Event{Type: runtime.EventButtonAction, Tag: tag, UniqueID: uniqueID, Index: index, Action: mappedAction})
}

func normalizeButtonMode(mode string) string {
	switch mode {
	case "single", "dimmer", "scene", "default":
		return mode
	default:
		return ""
	}
}

func mapButtonActionByMode(mode string, index int, action string) string {
	if mode != "scene" {
		return action
	}
	switch action {
	case "tip":
		return "scene5"
	case "tip2":
		return "scene0"
	case "tip3":
		return "scene18"
	case "tip4":
		return "scene17"
	case "hold", "dimup", "dimdown":
		if index%2 == 0 {
			return "scene43"
		}
		return "scene42"
	default:
		return action
	}
}

func buttonClickAction(clickCount int) string {
	switch {
	case clickCount <= 1:
		return "tip"
	case clickCount == 2:
		return "tip2"
	case clickCount == 3:
		return "tip3"
	default:
		return "tip4"
	}
}

func buttonRepeatCount(pressDuration time.Duration) int {
	extra := pressDuration - buttonHoldThreshold
	if extra <= 0 {
		return 1
	}
	return 1 + int(extra/buttonDimRepeatInterval)
}

func buttonDimRepeatAction(index int) string {
	if index%2 == 0 {
		return "dimup"
	}
	return "dimdown"
}
