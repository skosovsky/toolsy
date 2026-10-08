package toolsy

import (
	"errors"
	"unicode/utf8"

	"github.com/skosovsky/toolsy/internal/jsonschemax"
)

const (
	EventControl EventType = "control"
)

// ControlSignal is a sealed control-plane marker. Implementations live in this package only.
type ControlSignal interface {
	isControlSignal()
}

// PauseSignal asks the host to pause its continuation after this call.
// It does not persist a continuation, block other calls or issue approval authority.
type PauseSignal struct {
	Reason string
}

func (*PauseSignal) isControlSignal() {}

// YieldSignal asks the host to end its continuation quietly after this call.
// It is not a successful terminal result or a scheduler instruction.
type YieldSignal struct {
	Result string
}

func (*YieldSignal) isControlSignal() {}

// HaltSignal asks the host to stop its continuation after this call.
// It does not cancel this context, other calls or a registry/session.
type HaltSignal struct {
	Reason string
}

func (*HaltSignal) isControlSignal() {}

// HostEventSignal carries named data for a host-owned event adapter.
// Name has no built-in meaning; delivery never executes an action or grants authority.
// PayloadJSON is optional strict UTF-8 JSON, bounded with Name by MaxControlBytes.
type HostEventSignal struct {
	Name        string
	PayloadJSON []byte
}

func (*HostEventSignal) isControlSignal() {}

// Control errors are not tool execution failures; middleware must pass them through unchanged.
var (
	ErrPause     = errors.New("toolsy: control pause")
	ErrYield     = errors.New("toolsy: control yield")
	ErrHalt      = errors.New("toolsy: control halt")
	ErrHostEvent = errors.New("toolsy: control host event")
)

// ControlErrorFromSignal maps a control signal to its sentinel error for Execute return values.
func ControlErrorFromSignal(sig ControlSignal) error {
	if sig == nil {
		return nil
	}
	if _, err := controlSize(sig); err != nil {
		return err
	}
	switch sig.(type) {
	case *PauseSignal:
		return ErrPause
	case *YieldSignal:
		return ErrYield
	case *HaltSignal:
		return ErrHalt
	case *HostEventSignal:
		return ErrHostEvent
	default:
		return nil
	}
}

// IsControlError reports whether err is a control-plane signal error.
// Diagnostic control causes inside known contract/outcome failures are not signals.
func IsControlError(err error) bool {
	if requiresOutcomeReconciliation(err) {
		return false
	}
	return errors.Is(err, ErrPause) ||
		errors.Is(err, ErrYield) ||
		errors.Is(err, ErrHalt) ||
		errors.Is(err, ErrHostEvent)
}

// YieldControl validates and snapshots a control, delivers it, then returns its sentinel.
// Return this error from the producer; a producer ignoring it is not preempted.
func YieldControl(yield func(Chunk) error, sig ControlSignal) error {
	if _, err := controlSize(sig); err != nil {
		return err
	}
	if yield == nil {
		return invalidControl("nil delivery callback")
	}
	ctrlErr := ControlErrorFromSignal(sig)
	var c Chunk
	c.Event = EventControl
	c.Control = cloneControl(sig)
	if err := yield(c); err != nil {
		return err
	}
	return ctrlErr
}

// MaxControlBytes bounds the sum of text/name/payload field bytes per control and
// per result control list. It is a delivery contract, not a producer allocation cap.
const MaxControlBytes = 64 * 1024

// MaxControlSignals bounds controls declared on one terminal result.
const MaxControlSignals = 64

// MaxHostEventNameBytes bounds a host event's ASCII routing name.
const MaxHostEventNameBytes = 128

func invalidControl(reason string) error {
	return NewInternalError(&ResultContractError{Kind: "control_contract", Cause: errors.New(reason)})
}

func controlSize(signal ControlSignal) (int, error) {
	var text string
	var payload []byte
	switch value := signal.(type) {
	case *PauseSignal:
		if value == nil {
			return 0, invalidControl("nil pause signal")
		}
		text = value.Reason
	case *YieldSignal:
		if value == nil {
			return 0, invalidControl("nil yield signal")
		}
		text = value.Result
	case *HaltSignal:
		if value == nil {
			return 0, invalidControl("nil halt signal")
		}
		text = value.Reason
	case *HostEventSignal:
		if value == nil {
			return 0, invalidControl("nil host event signal")
		}
		if !validHostEventName(value.Name) {
			return 0, invalidControl("invalid host event name")
		}
		text, payload = value.Name, value.PayloadJSON
	default:
		return 0, invalidControl("missing or unsupported control signal")
	}
	if len(text) > MaxControlBytes || len(payload) > MaxControlBytes-len(text) {
		return 0, invalidControl("control field byte limit exceeded")
	}
	if !utf8.ValidString(text) || !utf8.Valid(payload) {
		return 0, invalidControl("control fields must be UTF-8")
	}
	if len(payload) != 0 {
		if _, err := jsonschemax.Decode(payload); err != nil {
			return 0, NewInternalError(&ResultContractError{Kind: "control_contract", Cause: err})
		}
	}
	return len(text) + len(payload), nil
}

func validHostEventName(name string) bool {
	if len(name) == 0 || len(name) > MaxHostEventNameBytes {
		return false
	}
	for i, c := range []byte(name) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			continue
		}
		if i > 0 && (c == '.' || c == '_' || c == '-' || c == '/' || c == ':') {
			continue
		}
		return false
	}
	return true
}

func validateControlDeclarations(c Chunk) error {
	if len(c.Controls) > MaxControlSignals {
		return invalidControl("control count limit exceeded")
	}
	if c.Event != EventResult && len(c.Controls) != 0 {
		return invalidControl("controls require a terminal result")
	}
	total := 0
	for _, signal := range c.Controls {
		size, err := controlSize(signal)
		if err != nil {
			return err
		}
		if size > MaxControlBytes-total {
			return invalidControl("control list byte limit exceeded")
		}
		total += size
	}
	if c.Event == EventControl {
		if len(c.Data) != 0 || c.MimeType != "" || c.IsError {
			return invalidControl("control chunk cannot carry wire/error data")
		}
		_, err := controlSize(c.Control)
		return err
	}
	if c.Control != nil {
		return invalidControl("singular control requires control event")
	}
	return nil
}

func cloneControl(signal ControlSignal) ControlSignal {
	switch value := signal.(type) {
	case *PauseSignal:
		return &PauseSignal{Reason: value.Reason}
	case *YieldSignal:
		return &YieldSignal{Result: value.Result}
	case *HaltSignal:
		return &HaltSignal{Reason: value.Reason}
	case *HostEventSignal:
		return &HostEventSignal{Name: value.Name, PayloadJSON: append([]byte(nil), value.PayloadJSON...)}
	default:
		return nil
	}
}
