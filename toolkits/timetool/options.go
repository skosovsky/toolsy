package timetool

import "time"

// Option configures AsTools (location for local time, tool names and descriptions).
type Option func(*options)

type options struct {
	location      *time.Location
	currentName   string
	currentDesc   string
	calculateName string
	calculateDesc string
}

const (
	defaultCurrentName   = "time_current"
	defaultCurrentDesc   = "Get current time in UTC and local timezone with weekday"
	defaultCalculateName = "time_calculate"
	defaultCalculateDesc = "Add days or hours to a date (DST-safe); returns result in RFC3339"
)

func applyDefaults(o *options) {
	if o.location == nil {
		o.location = time.Local
		if o.location == nil {
			o.location = time.UTC
		}
	}
	if o.currentName == "" {
		o.currentName = defaultCurrentName
	}
	if o.currentDesc == "" {
		o.currentDesc = defaultCurrentDesc
	}
	if o.calculateName == "" {
		o.calculateName = defaultCalculateName
	}
	if o.calculateDesc == "" {
		o.calculateDesc = defaultCalculateDesc
	}
}

// WithLocation sets the timezone for "local" in time_current (default: system local, fallback UTC).
func WithLocation(loc *time.Location) Option {
	return func(o *options) {
		o.location = loc
	}
}

// WithCurrentName sets the name of the time_current tool.
func WithCurrentName(name string) Option {
	return func(o *options) {
		o.currentName = name
	}
}

// WithCurrentDescription sets the description of the time_current tool.
func WithCurrentDescription(desc string) Option {
	return func(o *options) {
		o.currentDesc = desc
	}
}

// WithCalculateName sets the name of the time_calculate tool.
func WithCalculateName(name string) Option {
	return func(o *options) {
		o.calculateName = name
	}
}

// WithCalculateDescription sets the description of the time_calculate tool.
func WithCalculateDescription(desc string) Option {
	return func(o *options) {
		o.calculateDesc = desc
	}
}
