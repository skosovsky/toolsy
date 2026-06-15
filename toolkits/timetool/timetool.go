package timetool

import (
	"context"
	"fmt"
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

// ComputeCurrent returns current time fields for the given location.
func ComputeCurrent(loc *time.Location) CurrentResult {
	if loc == nil {
		loc = time.UTC
	}
	now := time.Now().UTC()
	local := now.In(loc)
	return CurrentResult{
		UTC:     now.Format(time.RFC3339),
		Local:   local.Format(time.RFC3339),
		Weekday: local.Weekday().String(),
		Unix:    now.Unix(),
	}
}

// AsTools returns two tools: time_current and time_calculate.
func AsTools(opts ...Option) ([]toolsy.Tool, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	applyDefaults(&o)

	var currentTool toolsy.Tool
	var err error
	if o.resultFormatter != nil || o.hostResultValidator != nil {
		currentTool, err = toolsy.NewTool[currentArgs, format.JSONResult](
			o.currentName,
			o.currentDesc,
			func(ctx context.Context, env *toolsy.RunEnv, _ currentArgs) (format.JSONResult, error) {
				loc, locErr := resolveLocation(ctx, env, &o)
				if locErr != nil {
					return format.JSONResult{}, locErr
				}
				raw, applyErr := format.ApplyWithEnvelope(
					ComputeCurrent(loc),
					func(v CurrentResult) CurrentResult { return v },
					o.resultFormatter,
					o.hostResultValidator,
					0,
				)
				if applyErr != nil {
					return format.JSONResult{}, applyErr
				}
				return format.JSONResult{Raw: raw}, nil
			},
			toolsy.WithReadOnly(),
		)
	} else {
		currentTool, err = toolsy.NewTool[currentArgs, CurrentResult](
			o.currentName,
			o.currentDesc,
			func(ctx context.Context, env *toolsy.RunEnv, _ currentArgs) (CurrentResult, error) {
				loc, locErr := resolveLocation(ctx, env, &o)
				if locErr != nil {
					return CurrentResult{}, locErr
				}
				return ComputeCurrent(loc), nil
			},
			toolsy.WithReadOnly(),
		)
	}
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
	if o.calculateFormatter != nil || o.hostResultValidator != nil {
		return toolsy.NewTool[calculateArgs, format.JSONResult](
			o.calculateName,
			o.calculateDesc,
			func(_ context.Context, _ *toolsy.RunEnv, args calculateArgs) (format.JSONResult, error) {
				res, calcErr := doCalculate(o, args)
				if calcErr != nil {
					return format.JSONResult{}, calcErr
				}
				raw, applyErr := format.ApplyWithEnvelope(
					res,
					func(v CalculateResult) CalculateResult { return v },
					o.calculateFormatter,
					o.hostResultValidator,
					0,
				)
				if applyErr != nil {
					return format.JSONResult{}, applyErr
				}
				return format.JSONResult{Raw: raw}, nil
			},
			toolsy.WithReadOnly(),
		)
	}
	return toolsy.NewTool[calculateArgs, CalculateResult](
		o.calculateName,
		o.calculateDesc,
		func(_ context.Context, _ *toolsy.RunEnv, args calculateArgs) (CalculateResult, error) {
			return doCalculate(o, args)
		},
		toolsy.WithReadOnly(),
	)
}

func resolveLocation(ctx context.Context, env *toolsy.RunEnv, o *options) (*time.Location, error) {
	if o.locationProvider != nil {
		return o.locationProvider(ctx, env)
	}
	return o.location, nil
}

func doCalculate(o *options, args calculateArgs) (CalculateResult, error) {
	if args.BaseDate == "" {
		return CalculateResult{}, toolsy.NewValidationError("base_date is required")
	}
	baseDate, err := time.Parse(time.RFC3339, args.BaseDate)
	if err != nil {
		return CalculateResult{}, toolsy.NewValidationError(
			"invalid base_date: must be RFC3339 (e.g. 2026-03-11T12:00:00Z)",
		)
	}
	inLoc := baseDate.In(o.location)
	result := inLoc.AddDate(0, 0, args.AddDays).Add(time.Duration(args.AddHours) * time.Hour)
	return CalculateResult{
		Result:  result.Format(time.RFC3339),
		Weekday: result.Weekday().String(),
	}, nil
}
