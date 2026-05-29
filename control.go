package toolsy

import (
	"errors"
)

const (
	EventControl EventType = "control"
)

// ControlSignal is a sealed control-plane marker. Implementations live in this package only.
type ControlSignal interface {
	isControlSignal()
}

// PauseSignal requests orchestrator-managed pause (human-in-the-loop).
type PauseSignal struct {
	Reason string
}

func (*PauseSignal) isControlSignal() {}

// YieldSignal requests silent early completion without treating the run as failed.
type YieldSignal struct {
	Result string
}

func (*YieldSignal) isControlSignal() {}

// HaltSignal requests hard stop of the current agent track.
type HaltSignal struct {
	Reason string
}

func (*HaltSignal) isControlSignal() {}

// UIActionSignal carries a typed UI action for the orchestrator shell.
type UIActionSignal struct {
	Action      string
	PayloadJSON []byte
}

func (*UIActionSignal) isControlSignal() {}

// Control errors are not tool execution failures; middleware must pass them through unchanged.
var (
	ErrPause = errors.New("toolsy: control pause")
	ErrYield = errors.New("toolsy: control yield")
	ErrHalt  = errors.New("toolsy: control halt")
)

// ControlErrorFromSignal maps a control signal to its sentinel error for Execute return values.
func ControlErrorFromSignal(sig ControlSignal) error {
	if sig == nil {
		return nil
	}
	switch sig.(type) {
	case *PauseSignal:
		return ErrPause
	case *YieldSignal:
		return ErrYield
	case *HaltSignal:
		return ErrHalt
	default:
		return nil
	}
}

// IsControlError reports whether err is a control-plane signal error.
func IsControlError(err error) bool {
	return errors.Is(err, ErrPause) ||
		errors.Is(err, ErrYield) ||
		errors.Is(err, ErrHalt)
}

// YieldControl emits a typed control chunk and returns the matching control error.
func YieldControl(yield func(Chunk) error, sig ControlSignal) error {
	if sig == nil {
		return &SystemError{Err: errors.New("toolsy: nil control signal")}
	}
	c := Chunk{
		Event:   EventControl,
		Control: sig,
	}
	if err := yield(c); err != nil {
		return err
	}
	if ctrlErr := ControlErrorFromSignal(sig); ctrlErr != nil {
		return ctrlErr
	}
	return nil
}
