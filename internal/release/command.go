package release

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type commands struct {
	env     []string
	private bool
}

func (c commands) run(ctx context.Context, dir, name string, args ...string) (string, error) {
	return c.runInput(ctx, dir, nil, name, args...)
}

func (c commands) runInput(
	ctx context.Context,
	dir string,
	input io.Reader,
	name string,
	args ...string,
) (string, error) {
	if name == "git" {
		overrides := []string{"-c", "core.fsmonitor=false"}
		if c.private {
			overrides = append(overrides, "-c", "core.autocrlf=false", "-c", "core.eol=lf", "-c", "core.symlinks=true")
		}
		args = append(overrides, args...)
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdin = input
	cmd.Env = append(commandEnvironment(c.private), c.env...)
	configureCommand(cmd)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	finishCommand(ctx, cmd)
	if ctx.Err() != nil {
		err = errors.Join(err, ctx.Err())
	}
	if err != nil {
		return "", fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, output.String())
	}
	if strings.HasSuffix(output.String(), "\x00") {
		return output.String(), nil
	}
	return strings.TrimSpace(output.String()), nil
}

func commandEnvironment(private bool) []string {
	var environment []string
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if key == "GIT_CONFIG_COUNT" || key == "GIT_CONFIG_PARAMETERS" || strings.HasPrefix(key, "GIT_CONFIG_KEY_") ||
			strings.HasPrefix(key, "GIT_CONFIG_VALUE_") {
			continue
		}
		switch key {
		case "GIT_DIR",
			"GIT_WORK_TREE",
			"GIT_INDEX_FILE",
			"GIT_OBJECT_DIRECTORY",
			"GIT_ALTERNATE_OBJECT_DIRECTORIES",
			"GIT_COMMON_DIR",
			"GIT_NAMESPACE":
			continue
		default:
			environment = append(environment, item)
		}
	}
	return append(environment, countedGitConfiguration(private)...)
}

func countedGitConfiguration(private bool) []string {
	count, err := strconv.Atoi(os.Getenv("GIT_CONFIG_COUNT"))
	if err != nil || count < 0 || count > len(os.Environ()) {
		return nil
	}
	var values []string
	kept := 0
	for i := range count {
		key := os.Getenv("GIT_CONFIG_KEY_" + strconv.Itoa(i))
		if private &&
			(strings.EqualFold(key, "core.autocrlf") || strings.EqualFold(key, "core.eol") || strings.EqualFold(key, "core.symlinks")) {
			continue
		}
		switch strings.ToLower(key) {
		case "core.worktree",
			"core.bare",
			"core.hookspath",
			"core.fsmonitor",
			"extensions.worktreeconfig":
			continue
		}
		values = append(
			values,
			"GIT_CONFIG_KEY_"+strconv.Itoa(kept)+"="+key,
			"GIT_CONFIG_VALUE_"+strconv.Itoa(kept)+"="+os.Getenv("GIT_CONFIG_VALUE_"+strconv.Itoa(i)),
		)
		kept++
	}
	return append(values, "GIT_CONFIG_COUNT="+strconv.Itoa(kept))
}
