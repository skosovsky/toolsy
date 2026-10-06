// Package release prepares and verifies committed module artifacts in a private
// checkout before publication. Supported release hosts are Linux and macOS.
package release

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	modmodule "golang.org/x/mod/module"
)

// Config describes one release of the committed HEAD, without source mutations.
type Config struct {
	Dir         string
	Mode        string
	Modules     string
	PrepareOnly bool
	FullChecks  bool
	Input       io.ReadCloser
	Output      io.Writer
}

const moduleVerifyTimeout = 10 * time.Minute
const fullChecksTimeout = 30 * time.Minute
const releaseBreakMode = "break"
const releaseRemote = "origin"
const gitQuiet = "--quiet"
const gitConfig = "config"

type candidate struct {
	source, checkout, temp, commit, root, version string
	modules                                       []*module
	commands                                      commands
}

// Run verifies artifacts before requesting final publication confirmation.
func Run(ctx context.Context, cfg Config) (err error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return errors.New("release requires macOS or Linux")
	}
	if os.Getenv("GIT_CONFIG_PARAMETERS") != "" {
		return errors.New("release does not support GIT_CONFIG_PARAMETERS; use Git config or counted auth settings")
	}
	if cfg.Mode != "patch" && cfg.Mode != releaseBreakMode {
		return errors.New("release mode must be patch or break")
	}
	if cfg.Input == nil || cfg.Output == nil {
		return errors.New("release requires input and output")
	}
	candidate, err := isolate(ctx, cfg.Dir, cfg.Modules)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, removeCandidate(candidate.temp)) }()
	if err = candidate.prepare(ctx, cfg.Mode, cfg.Output); err != nil {
		return err
	}
	if cfg.FullChecks {
		if err = candidate.fullChecks(ctx, cfg.Mode); err != nil {
			return err
		}
	}
	status, statusErr := candidate.commands.run(
		ctx,
		candidate.checkout,
		"git",
		"status",
		"--porcelain",
		"--untracked-files=no",
	)
	if statusErr != nil {
		return statusErr
	}
	if status != "" {
		return errors.New("release checks changed tracked candidate files")
	}
	if cfg.PrepareOnly {
		return nil
	}
	if err = confirmPublication(ctx, cfg.Input, cfg.Output); err != nil {
		return err
	}

	return candidate.publish(ctx, cfg.Output)
}

func isolate(ctx context.Context, dir, moduleArg string) (*candidate, error) {
	read := commands{env: []string{"GIT_OPTIONAL_LOCKS=0", "GOWORK=off"}, private: false}
	source, err := read.run(ctx, dir, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	status, err := read.run(ctx, source, "git", "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return nil, err
	}
	if status != "" {
		return nil, errors.New("commit or stash tracked changes before release")
	}
	commit, err := read.run(ctx, source, "git", "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	files, err := read.run(ctx, source, "git", "ls-tree", "-r", "-z", "--name-only", commit)
	if err != nil {
		return nil, err
	}
	dirs, err := moduleInventory(files, moduleArg)
	if err != nil {
		return nil, err
	}
	fetch, err := read.run(ctx, source, "git", "remote", "get-url", releaseRemote)
	if err != nil {
		return nil, err
	}
	push, err := read.run(ctx, source, "git", "remote", "get-url", "--push", "--all", releaseRemote)
	if err != nil {
		return nil, err
	}
	if push == "" || strings.Contains(push, "\n") {
		return nil, errors.New("release requires exactly one push destination for atomic publication")
	}
	temp, err := os.MkdirTemp("", "toolsy-release-")
	if err != nil {
		return nil, err
	}
	checkout := filepath.Join(temp, "checkout")
	c := &candidate{
		source:   source,
		checkout: checkout,
		temp:     temp,
		commit:   commit,
		commands: read,
		root:     "",
		version:  "",
		modules:  nil,
	}
	c.commands.private = true
	if err = c.clone(ctx, resolveRemote(source, fetch), resolveRemote(source, push), files); err != nil {
		return nil, errors.Join(err, removeCandidate(temp))
	}
	for _, dir := range dirs {
		m, loadErr := c.loadModule(ctx, dir)
		if loadErr != nil {
			return nil, errors.Join(loadErr, removeCandidate(temp))
		}
		c.modules = append(c.modules, m)
	}
	return c, nil
}

func (c *candidate) clone(ctx context.Context, fetch, push, files string) error {
	if err := os.MkdirAll(filepath.Join(c.temp, "hooks"), 0o700); err != nil {
		return err
	}
	steps := [][]string{
		{"clone", "--no-local", "--no-hardlinks", gitQuiet, "--no-checkout", c.source, c.checkout},
		{"-C", c.checkout, gitConfig, "core.hooksPath", filepath.Join(c.temp, "hooks")},
		{"-C", c.checkout, gitConfig, "core.worktree", c.checkout},
		{"-C", c.checkout, gitConfig, "core.autocrlf", "false"},
		{"-C", c.checkout, gitConfig, "core.eol", "lf"},
		{"-C", c.checkout, gitConfig, "core.symlinks", "true"},
		{"-C", c.checkout, "read-tree", c.commit},
		{"-C", c.checkout, "checkout", "--force", "--detach", gitQuiet, c.commit},
		{"-C", c.checkout, "remote", "set-url", releaseRemote, fetch},
		{"-C", c.checkout, "remote", "set-url", "--push", releaseRemote, push},
		{"-C", c.checkout, "fetch", "--tags", gitQuiet, releaseRemote},
	}
	for _, args := range steps {
		if len(args) > 2 && args[2] == "checkout" {
			if err := c.validateAttributes(ctx, files); err != nil {
				return err
			}
		}
		if _, err := c.commands.run(ctx, c.temp, "git", args...); err != nil {
			return err
		}
	}
	for _, key := range []string{"user.name", "user.email", "commit.gpgsign", "gpg.format", "user.signingkey"} {
		value, err := c.commands.run(ctx, c.source, "git", gitConfig, "--get", key)
		if err != nil {
			var exit *exec.ExitError
			if errors.As(err, &exit) && exit.ExitCode() == 1 {
				continue
			}
			return err
		}
		if _, err = c.commands.run(ctx, c.checkout, "git", gitConfig, key, value); err != nil {
			return err
		}
	}
	return nil
}

func moduleInventory(files, moduleArg string) ([]string, error) {
	var dirs []string
	for file := range strings.SplitSeq(files, "\x00") {
		if filepath.Base(file) == "go.mod" {
			dirs = append(dirs, filepath.ToSlash(filepath.Dir(file)))
		}
	}
	sort.Strings(dirs)
	if len(dirs) == 0 || dirs[0] != "." {
		return nil, errors.New("committed root go.mod is required")
	}
	supplied := strings.Fields(moduleArg)
	if len(supplied) == 0 {
		return dirs, nil
	}
	for i, dir := range supplied {
		clean := filepath.ToSlash(filepath.Clean(dir))
		if filepath.IsAbs(dir) || clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, errors.New("module paths must stay in the repository")
		}
		supplied[i] = clean
	}
	sort.Strings(supplied)
	if strings.Join(dirs, "\n") != strings.Join(supplied, "\n") {
		return nil, errors.New("supplied module list must equal the committed go.mod inventory")
	}
	return dirs, nil
}

func resolveRemote(source, remote string) string {
	if filepath.IsAbs(remote) || strings.Contains(remote, ":") {
		return remote
	}
	return filepath.Join(source, remote)
}

func (c *candidate) prepare(ctx context.Context, mode string, out io.Writer) error {
	c.root = c.modules[0].path
	latest, err := c.commands.run(ctx, c.checkout, "git", "tag", "-l", "v*")
	if err != nil {
		return err
	}
	c.version, err = nextVersion(latest, mode)
	if err != nil {
		return err
	}
	if err = c.checkTagCollisions(ctx); err != nil {
		return err
	}
	ordered, err := orderModules(c.modules, c.root)
	if err != nil {
		return err
	}
	for _, m := range ordered {
		if err = c.rewrite(ctx, m); err != nil {
			return err
		}
	}
	cache, err := c.commands.run(ctx, c.checkout, "go", "env", "GOMODCACHE")
	if err != nil {
		return err
	}
	proxy := filepath.Join(c.temp, "proxy")
	fallback, err := c.commands.run(ctx, c.checkout, "go", "env", "GOPROXY")
	if err != nil {
		return err
	}
	noProxy, err := c.commands.run(ctx, c.checkout, "go", "env", "GONOPROXY")
	if err != nil {
		return err
	}
	if slices.ContainsFunc(c.modules, func(m *module) bool { return matchesModulePattern(noProxy, m.path) }) {
		// Overlapping private namespaces cannot bypass the owned artifact proxy.
		// Use cached archives or direct VCS for every external dependency, so
		// disabling the bypass never sends private names to a public proxy.
		fallback = "direct"
		noProxy = "none"
	}
	noSum, err := c.commands.run(ctx, c.checkout, "go", "env", "GONOSUMDB")
	if err != nil {
		return err
	}
	c.commands.env = append(c.commands.env,
		"GONOPROXY="+noProxy,
		"GOPATH="+filepath.Join(c.temp, "gopath"),
		"GOMODCACHE="+filepath.Join(c.temp, "modcache"),
		"GOPROXY="+fileURL(proxy)+","+fileURL(filepath.Join(cache, "cache", "download"))+","+fallback,
		"GONOSUMDB="+noSum+","+c.root+","+c.root+"/*", "GOTOOLCHAIN=local")
	files, err := c.commands.run(ctx, c.checkout, "git", "ls-files", "-z")
	if err != nil {
		return err
	}
	for _, m := range ordered {
		if _, err = fmt.Fprintf(out, "Verify %s@%s (GOWORK=off)\n", m.path, c.version); err != nil {
			return err
		}
		verifyCtx, cancel := context.WithTimeout(ctx, moduleVerifyTimeout)
		err = c.verifyModule(verifyCtx, m, proxy, strings.Split(files, "\x00"))
		cancel()
		if err != nil {
			return err
		}
	}
	return c.commitCandidate(ctx, out)
}

func fileURL(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

var releaseVersion = regexp.MustCompile(`^v([0-9]+)\.([0-9]+)\.([0-9]+)$`)

func nextVersion(tags, mode string) (string, error) {
	best := [3]uint64{}
	for tag := range strings.FieldsSeq(tags) {
		current, ok, err := parseReleaseVersion(tag)
		if err != nil {
			return "", err
		}
		if ok && slices.Compare(current[:], best[:]) > 0 {
			best = current
		}
	}

	index := 2
	if mode == releaseBreakMode {
		index = 0
		if best[0] == 0 {
			index = 1
		}
		for i := index + 1; i < 3; i++ {
			best[i] = 0
		}
	}
	if best[index] == ^uint64(0) {
		return "", errors.New("release version overflow")
	}
	best[index]++
	return fmt.Sprintf("v%d.%d.%d", best[0], best[1], best[2]), nil
}

func (c *candidate) commitCandidate(ctx context.Context, out io.Writer) error {
	args := []string{"add", "--"}
	for _, m := range c.modules {
		args = append(args, filepath.Join(m.dir, "go.mod"))
		sum := filepath.Join(m.dir, "go.sum")
		if _, err := os.Stat(filepath.Join(c.checkout, sum)); err == nil {
			args = append(args, sum)
		}
	}
	if _, err := c.commands.run(ctx, c.checkout, "git", args...); err != nil {
		return err
	}
	changed, err := c.commands.run(ctx, c.checkout, "git", "diff", "--cached", "--name-only")
	if err != nil {
		return err
	}
	if changed != "" {
		if _, err = c.commands.run(ctx, c.checkout, "git", "commit", "-m", "chore: release "+c.version); err != nil {
			return err
		}
	}
	hash, err := c.commands.run(ctx, c.checkout, "git", "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Verified candidate %s %s from %s\n", c.version, hash, c.commit)
	return err
}

func (c *candidate) publish(ctx context.Context, out io.Writer) error {
	if err := c.checkTagCollisions(ctx); err != nil {
		return err
	}
	refs := c.releaseRefs()
	for _, ref := range refs {
		if _, err := c.commands.run(
			ctx,
			c.checkout,
			"git",
			"tag",
			"--",
			strings.TrimPrefix(ref, "refs/tags/"),
		); err != nil {
			return err
		}
	}
	args := []string{"-c", "remote.origin.mirror=false", "push", "--atomic", "--no-follow-tags", releaseRemote}
	for _, ref := range refs {
		args = append(args, ref+":"+ref)
	}
	if _, err := c.commands.run(ctx, c.checkout, "git", args...); err != nil {
		return err
	}
	_, err := fmt.Fprintf(out, "Published %s; source checkout unchanged\n", c.version)
	return err
}

// The input is owned by this release invocation; Close must unblock Read.
func confirmPublication(ctx context.Context, input io.ReadCloser, output io.Writer) error {
	if _, err := fmt.Fprint(output, "Publish verified candidate? [y/N] "); err != nil {
		return err
	}
	type reply struct {
		answer string
		err    error
	}
	responses := make(chan reply, 1)
	go func() { answer, err := bufio.NewReader(input).ReadString('\n'); responses <- reply{answer, err} }()
	select {
	case <-ctx.Done():
		_ = input.Close()
		select {
		case <-responses:
		case <-time.After(time.Second):
		}
		return ctx.Err()
	case response := <-responses:
		if response.err != nil && !errors.Is(response.err, io.EOF) {
			return response.err
		}
		if strings.TrimSpace(response.answer) != "y" && strings.TrimSpace(response.answer) != "Y" {
			return errors.New("publication aborted")
		}
		return ctx.Err()
	}
}

func matchesModulePattern(patterns, module string) bool {
	return modmodule.MatchPrefixPatterns(patterns, module)
}

func parseReleaseVersion(tag string) ([3]uint64, bool, error) {
	parts := releaseVersion.FindStringSubmatch(tag)
	var current [3]uint64
	if parts == nil {
		return current, false, nil
	}
	for i := range 3 {
		number, err := strconv.ParseUint(parts[i+1], 10, 64)
		if err != nil {
			return current, false, err
		}
		current[i] = number
	}
	return current, true, nil
}

func removeCandidate(dir string) error {
	walkErr := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			//nolint:gosec // Invocation-owned directories need search permission for removal.
			return os.Chmod(path, 0o700)
		}
		return nil
	})
	return errors.Join(walkErr, os.RemoveAll(dir))
}

func (c *candidate) fullChecks(ctx context.Context, mode string) error {
	checksCtx, cancel := context.WithTimeout(ctx, fullChecksTimeout)
	defer cancel()
	if _, err := os.Stat(filepath.Join(c.checkout, "Makefile")); err == nil {
		if _, err = c.commands.run(checksCtx, c.checkout, "make", "lint", "test"); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if mode == releaseBreakMode {
		preflight := filepath.Join(c.checkout, "scripts", "task34-preflight.sh")
		if _, err := os.Stat(preflight); err == nil {
			if _, err = c.commands.run(checksCtx, c.checkout, "bash", preflight); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
