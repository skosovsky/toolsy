//go:build !darwin && !linux

package filejournal

import (
	"context"
	"errors"
	"os"
)

func platformSupported() error                              { return errors.New("filejournal: platform unsupported") }
func lockJournal(context.Context, string) (*os.File, error) { return nil, platformSupported() }
