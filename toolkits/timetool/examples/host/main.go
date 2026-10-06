// The host explicitly resolves timezone state; timetool has no automatic state integration.
package main

import (
	"context"
	"errors"
	"fmt"
	"time"
	_ "time/tzdata"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/timetool"
)

const (
	timezoneKey      = "host.timezone"
	maxTimezoneBytes = 64
)

// fixtureStore is immutable local test data, not a durable storage implementation.
type fixtureStore struct{ zone string }

func (s fixtureStore) Load(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if key != timezoneKey {
		return nil, errors.New("host: unknown state key")
	}
	return []byte(s.zone), nil
}
func (fixtureStore) Save(context.Context, string, []byte) error {
	return errors.New("host: fixture store is read-only")
}

func loadLocation(ctx context.Context, env *toolsy.RunEnv) (*time.Location, error) {
	if env == nil || env.StateStore == nil {
		return nil, errors.New("host: timezone StateStore is required")
	}
	raw, err := env.StateStore.Load(ctx, timezoneKey)
	if err != nil {
		return nil, fmt.Errorf("host: load timezone: %w", err)
	}
	// Returned-byte check does not bound arbitrary allocations inside host Load.
	if len(raw) > maxTimezoneBytes {
		return nil, errors.New("host: timezone value exceeds 64 bytes")
	}
	zone := string(raw)
	if zone != "America/New_York" && zone != "UTC" {
		return nil, errors.New("host: unsupported timezone")
	}
	return time.LoadLocation(zone)
}

func main() {
	if err := run(context.Background()); err != nil {
		panic(err)
	}
}

func run(ctx context.Context) error {
	tools, err := timetool.AsTools(timetool.WithLocation(time.UTC), timetool.WithLocationProvider(loadLocation))
	if err != nil {
		return err
	}
	env := toolsy.NewRunEnv(nil, toolsy.WithStateStore(fixtureStore{zone: "America/New_York"}))
	for _, input := range []string{
		`{"base_date":"2026-03-07T17:00:00Z","add_days":1,"add_hours":0}`,
		`{"base_date":"2026-03-07T17:00:00Z","add_days":0,"add_hours":24}`,
	} {
		if callErr := tools[1].Execute(ctx, env, toolsy.ToolInput{ArgsJSON: []byte(input)},
			func(chunk toolsy.Chunk) error { fmt.Println(string(chunk.Data)); return nil },
		); callErr != nil {
			return callErr
		}
	}
	return nil
}
