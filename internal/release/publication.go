package release

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

func (c *candidate) releaseRefs() []string {
	refs := make([]string, 0, len(c.modules))
	for _, m := range c.modules {
		tag := c.version
		if m.dir != "." {
			tag = m.dir + "/" + tag
		}
		refs = append(refs, "refs/tags/"+tag)
	}
	return refs
}

// Check the publication destination before preparation and again after approval.
// Git's atomic, non-forcing push arbitrates concurrent conflicting updates.
func (c *candidate) checkTagCollisions(ctx context.Context) error {
	urls, err := c.commands.run(ctx, c.checkout, "git", "remote", "get-url", "--push", "--all", releaseRemote)
	if err != nil {
		return err
	}
	destinations := strings.Split(urls, "\n")
	if len(destinations) != 1 || destinations[0] == "" {
		return errors.New("release requires exactly one push destination for atomic publication")
	}
	remote, err := c.commands.run(ctx, c.checkout, "git", "ls-remote", "--refs", "--tags", destinations[0])
	if err != nil {
		return err
	}
	existing := make(map[string]bool)
	for line := range strings.SplitSeq(remote, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 {
			existing[fields[1]] = true
		}
	}
	for _, ref := range c.releaseRefs() {
		if _, err = c.commands.run(ctx, c.checkout, "git", "check-ref-format", ref); err != nil {
			return err
		}
		local, localErr := c.commands.run(
			ctx,
			c.checkout,
			"git",
			"tag",
			"--list",
			strings.TrimPrefix(ref, "refs/tags/"),
		)
		if localErr != nil {
			return localErr
		}
		if local != "" || existing[ref] {
			return fmt.Errorf("release tag already exists: %s", ref)
		}
	}
	return nil
}
