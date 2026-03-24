package timetool

import (
	"context"
	"fmt"
	"time"

	"github.com/skosovsky/toolsy"
)

type currentArgs struct{}

type currentResult struct {
	UTC     string `json:"utc"`
	Local   string `json:"local"`
	Weekday string `json:"weekday"`
	Unix    int64  `json:"unix"`
}

type calculateArgs struct {
	BaseDate string `json:"base_date"`
	AddDays  int    `json:"add_days"`
	AddHours int    `json:"add_hours"`
}

type calculateResult struct {
	Result  string `json:"result"`
	Weekday string `json:"weekday"`
}

// AsTools returns two tools: time_current and time_calculate.
func AsTools(opts ...Option) ([]toolsy.Tool, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	applyDefaults(&o)

	currentTool, err := toolsy.NewTool[currentArgs, currentResult](
		o.currentName,
		o.currentDesc,
		func(_ context.Context, _ currentArgs) (currentResult, error) {
			return doCurrent(&o)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("toolkit/timetool: build current tool: %w", err)
	}

	calculateTool, err := toolsy.NewTool[calculateArgs, calculateResult](
		o.calculateName,
		o.calculateDesc,
		func(_ context.Context, args calculateArgs) (calculateResult, error) {
			return doCalculate(&o, args)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("toolkit/timetool: build calculate tool: %w", err)
	}

	return []toolsy.Tool{currentTool, calculateTool}, nil
}

func doCurrent(o *options) (currentResult, error) {
	now := time.Now().UTC()
	local := now.In(o.location)
	return currentResult{
		UTC:     now.Format(time.RFC3339),
		Local:   local.Format(time.RFC3339),
		Weekday: local.Weekday().String(),
		Unix:    now.Unix(),
	}, nil
}

func doCalculate(o *options, args calculateArgs) (calculateResult, error) {
	if args.BaseDate == "" {
		return calculateResult{}, &toolsy.ClientError{
			Reason:    "base_date is required",
			Retryable: false,
			Err:       toolsy.ErrValidation,
		}
	}
	baseDate, err := time.Parse(time.RFC3339, args.BaseDate)
	if err != nil {
		return calculateResult{}, &toolsy.ClientError{
			Reason:    "invalid base_date: must be RFC3339 (e.g. 2026-03-11T12:00:00Z)",
			Retryable: false,
			Err:       toolsy.ErrValidation,
		}
	}
	// DST-safe: perform arithmetic in configured location so calendar days respect DST boundaries
	inLoc := baseDate.In(o.location)
	result := inLoc.AddDate(0, 0, args.AddDays).Add(time.Duration(args.AddHours) * time.Hour)
	return calculateResult{
		Result:  result.Format(time.RFC3339),
		Weekday: result.Weekday().String(),
	}, nil
}
