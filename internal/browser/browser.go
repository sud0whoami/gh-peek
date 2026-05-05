// Package browser opens URLs in the user's preferred web browser.
//
// The Opener interface is the seam used by the app router; production
// code uses OSOpener while tests inject CommandOpener with a recording
// Run function.
//
// URLs are validated before being passed to the platform launcher:
// only http(s) schemes are allowed. This is defensive against the
// rare case where an upstream URL is empty or otherwise hostile.
package browser

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
)

// Opener opens a URL in the user's web browser.
type Opener interface {
	Open(ctx context.Context, url string) error
}

// OSOpener launches the platform-default URL handler.
type OSOpener struct{}

// Open implements Opener using exec.CommandContext.
func (OSOpener) Open(ctx context.Context, url string) error {
	return CommandOpener{Run: defaultRun}.Open(ctx, url)
}

// CommandOpener calls Run with the platform-appropriate command and
// args. Tests inject a recording Run function.
type CommandOpener struct {
	Run func(ctx context.Context, name string, args ...string) error
}

// Open implements Opener.
func (c CommandOpener) Open(ctx context.Context, url string) error {
	if err := validateURL(url); err != nil {
		return err
	}
	name, args, err := commandFor(runtime.GOOS, url)
	if err != nil {
		return err
	}
	run := c.Run
	if run == nil {
		run = defaultRun
	}
	return run(ctx, name, args...)
}

// validateURL rejects empty input and anything that doesn't look like
// http(s). It does not perform full URL parsing — the goal is to
// refuse obviously unsafe inputs (javascript:, file:, empty) before
// they reach the OS launcher.
func validateURL(url string) error {
	if url == "" {
		return errors.New("browser: empty URL")
	}
	low := strings.ToLower(url)
	if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") {
		return fmt.Errorf("browser: refusing to open non-http(s) URL")
	}
	return nil
}

// commandFor returns the (name, args) pair to launch a URL on the
// given GOOS. Returns an error for unsupported platforms.
func commandFor(goos, url string) (string, []string, error) {
	switch goos {
	case "darwin":
		return "open", []string{url}, nil
	case "linux", "freebsd", "openbsd", "netbsd":
		return "xdg-open", []string{url}, nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}, nil
	default:
		return "", nil, fmt.Errorf("browser: unsupported GOOS %q", goos)
	}
}

// defaultRun is the production runner: exec.CommandContext, with
// stdout/stderr discarded so the launcher's noise does not pollute the
// TUI.
func defaultRun(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}
