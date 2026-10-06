package timetool

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/format"
)

// CurrentResult holds current time in UTC and local timezone.
type CurrentResult struct {
	UTC     string `json:"utc"`
	Local   string `json:"local"`
	Weekday string `json:"weekday"`
	Unix    int64  `json:"unix"`
}

type currentArgs struct{}

type calculateArgs struct {
	BaseDate string `json:"base_date"`
	AddDays  int    `json:"add_days"`
	AddHours int    `json:"add_hours"`
}

type CalculateResult struct {
	Result  string `json:"result"`
	Weekday string `json:"weekday"`
}

// ComputeCurrent returns current time fields for the given location (nil uses UTC).
// Locations whose offset cannot encode an exact RFC3339 instant return a validation error.
func ComputeCurrent(loc *time.Location) (CurrentResult, error) {
	if loc == nil {
		loc = time.UTC
	}
	now := time.Now().UTC()
	local := now.In(loc)
	if err := validateRFC3339Time(local); err != nil {
		return CurrentResult{}, err
	}
	return CurrentResult{
		UTC:     now.Format(time.RFC3339Nano),
		Local:   local.Format(time.RFC3339Nano),
		Weekday: local.Weekday().String(),
		Unix:    now.Unix(),
	}, nil
}

// AsTools returns two tools: time_current and time_calculate.
func AsTools(opts ...Option) ([]toolsy.Tool, error) {
	var o options
	for _, opt := range opts {
		if opt == nil {
			return nil, errors.New("toolkit/timetool: nil option")
		}
		opt(&o)
	}
	if o.maxWireBytes < 0 {
		return nil, errors.New("toolkit/timetool: limits must not be negative")
	}
	applyDefaults(&o)

	currentTool, err := toolsy.NewTool[currentArgs, format.JSONResult](o.currentName, o.currentDesc,
		func(ctx context.Context, env *toolsy.RunEnv, _ currentArgs) (format.JSONResult, error) {
			loc, err := resolveLocation(ctx, env, &o)
			if err != nil {
				return format.JSONResult{}, err
			}
			res, err := ComputeCurrent(loc)
			if err != nil {
				return format.JSONResult{}, err
			}
			raw, err := format.ApplyWithEnvelope(
				res,
				func(v CurrentResult) CurrentResult { return v },
				o.resultFormatter,
				o.hostResultValidator,
				o.maxWireBytes,
			)
			return format.JSONResult{Raw: raw}, err
		}, toolsy.WithReadOnly())

	if err != nil {
		return nil, fmt.Errorf("toolkit/timetool: build current tool: %w", err)
	}

	calculateTool, err := buildCalculateTool(&o)
	if err != nil {
		return nil, fmt.Errorf("toolkit/timetool: build calculate tool: %w", err)
	}

	return []toolsy.Tool{currentTool, calculateTool}, nil
}

func buildCalculateTool(o *options) (toolsy.Tool, error) {
	return toolsy.NewTool[calculateArgs, format.JSONResult](o.calculateName, o.calculateDesc,
		func(ctx context.Context, env *toolsy.RunEnv, args calculateArgs) (format.JSONResult, error) {
			loc, err := resolveLocation(ctx, env, o)
			if err != nil {
				return format.JSONResult{}, err
			}
			res, err := doCalculate(loc, args)
			if err != nil {
				return format.JSONResult{}, err
			}
			raw, err := format.ApplyWithEnvelope(
				res,
				func(v CalculateResult) CalculateResult { return v },
				o.calculateFormatter,
				o.hostResultValidator,
				o.maxWireBytes,
			)
			return format.JSONResult{Raw: raw}, err
		}, toolsy.WithReadOnly())
}

func resolveLocation(ctx context.Context, env *toolsy.RunEnv, o *options) (*time.Location, error) {
	if o.locationProvider != nil {
		loc, err := o.locationProvider(ctx, env)
		if err != nil {
			return nil, err
		}
		if loc == nil {
			return nil, toolsy.NewValidationError("location provider returned nil")
		}
		return loc, nil
	}
	return o.location, nil
}

func doCalculate(loc *time.Location, args calculateArgs) (CalculateResult, error) {
	if len(args.BaseDate) > maxBaseDateBytes {
		return CalculateResult{}, toolsy.NewValidationError("base_date exceeds 64 bytes")
	}
	if args.BaseDate == "" {
		return CalculateResult{}, toolsy.NewValidationError("base_date is required")
	}
	if !strictRFC3339.MatchString(args.BaseDate) {
		return CalculateResult{}, toolsy.NewValidationError(
			"invalid base_date: must use strict RFC3339 with at most 9 fractional digits",
		)
	}
	baseDate, err := time.Parse(time.RFC3339, args.BaseDate)
	if err != nil {
		return CalculateResult{}, toolsy.NewValidationError(
			"invalid base_date: must be RFC3339 (e.g. 2026-03-11T12:00:00Z)",
		)
	}
	// Calendar result must remain representable in RFC3339 (years 0 through 9999).
	if args.AddDays > 366*10000 || args.AddDays < -366*10000 {
		return CalculateResult{}, toolsy.NewValidationError("calendar addition overflows RFC3339")
	}
	const maxHours = (1<<63 - 1) / int64(time.Hour)
	if int64(args.AddHours) > maxHours || int64(args.AddHours) < -maxHours {
		return CalculateResult{}, toolsy.NewValidationError("hour duration overflows")
	}
	inLoc := baseDate.In(loc)
	calendar := inLoc.AddDate(0, 0, args.AddDays)
	result := calendar.Add(time.Duration(args.AddHours) * time.Hour)
	if calendar.Year() < 0 || calendar.Year() > 9999 || result.Year() < 0 || result.Year() > 9999 {
		return CalculateResult{}, toolsy.NewValidationError("result is outside RFC3339 year range")
	}
	if err := validateRFC3339Time(result); err != nil {
		return CalculateResult{}, err
	}
	return CalculateResult{
		Result:  result.Format(time.RFC3339Nano),
		Weekday: result.Weekday().String(),
	}, nil
}

// RFC3339 offsets have minute precision and hours below 24. Formatting a historical
// seconds offset would silently change the represented instant, so reject it.
func validateRFC3339Time(value time.Time) error {
	_, offset := value.Zone()
	if offset%60 != 0 || offset <= -24*60*60 || offset >= 24*60*60 {
		return toolsy.NewValidationError("location offset cannot represent the instant in RFC3339")
	}
	if value.Year() < 0 || value.Year() > 9999 {
		return toolsy.NewValidationError("result is outside RFC3339 year range")
	}
	return nil
}

// Go's parser accepts relaxed forms and silently discards precision beyond nanoseconds.
// Restrict input to our exact supported RFC3339 subset before interpreting the instant.
var strictRFC3339 = regexp.MustCompile(
	`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,9})?(Z|[+-]([01][0-9]|2[0-3]):[0-5][0-9])$`,
)
