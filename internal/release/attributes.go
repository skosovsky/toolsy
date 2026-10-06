package release

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Reject attributes that can transform checkout or archive bytes. Ordinary
// diff/merge/linguist metadata is supported; transformation requires a separate
// artifact-production contract rather than silently verifying different bytes.
func (c *candidate) validateAttributes(ctx context.Context, files string) error {
	raw, err := c.commands.runInput(ctx, c.checkout, strings.NewReader(files), "git",
		"check-attr", "--cached", "-z", "--stdin",
		"filter", "working-tree-encoding", "export-ignore", "export-subst", "ident", "crlf", "text", "eol")
	if err != nil {
		return err
	}
	fields := strings.Split(strings.TrimSuffix(raw, "\x00"), "\x00")
	if len(fields)%3 != 0 {
		return errors.New("invalid Git attribute response")
	}
	for i := 0; i < len(fields); i += 3 {
		value := fields[i+2]
		if value != "unspecified" && value != "unset" {
			return fmt.Errorf(
				"release does not support transforming Git attribute %s=%s on %s",
				fields[i+1],
				value,
				fields[i],
			)
		}
	}
	return nil
}
