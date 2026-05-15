// Package clipboard writes content to the system clipboard.
//
// The Copier interface is the seam used by the app layer; production
// code uses OSCopier while tests inject CommandCopier with a recording
// Run function.
//
// Platform resolution:
//   - darwin: pbcopy
//   - windows: clip.exe
//   - linux/other: wl-copy (when WAYLAND_DISPLAY is set), xclip, xsel
//
// ErrNoClipboardTool is returned on Linux when none of the supported
// tools are found on PATH.
package clipboard

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
)

// ErrNoClipboardTool is returned on Linux when none of the supported
// clipboard tools (wl-copy, xclip, xsel) are found on PATH.
var ErrNoClipboardTool = errors.New("clipboard: no supported tool found on PATH")

// Copier writes content to the system clipboard.
type Copier interface {
	Copy(ctx context.Context, content []byte) error
}

// OSCopier resolves the correct clipboard command for the current OS.
type OSCopier struct{}

// Copy implements Copier using the platform-appropriate command.
func (OSCopier) Copy(ctx context.Context, content []byte) error {
	cmd, args, err := resolveOSCommand()
	if err != nil {
		return err
	}
	return run(ctx, cmd, args, bytes.NewReader(content))
}

// resolveOSCommand returns the clipboard command and arguments for the
// current OS, or ErrNoClipboardTool on Linux when no tool is found.
func resolveOSCommand() (string, []string, error) {
	switch runtime.GOOS {
	case "darwin":
		return "pbcopy", nil, nil
	case "windows":
		return "clip.exe", nil, nil
	default: // linux and others
		return pickLinuxCommand(exec.LookPath, os.Getenv)
	}
}

// pickLinuxCommand is extracted so it can be unit-tested without
// touching the real PATH.
func pickLinuxCommand(
	lookPath func(string) (string, error),
	getenv func(string) string,
) (string, []string, error) {
	// Prefer wl-copy when running under Wayland.
	if getenv("WAYLAND_DISPLAY") != "" {
		if p, err := lookPath("wl-copy"); err == nil {
			return p, nil, nil
		}
	}
	if p, err := lookPath("xclip"); err == nil {
		return p, []string{"-selection", "clipboard"}, nil
	}
	if p, err := lookPath("xsel"); err == nil {
		return p, []string{"--clipboard", "--input"}, nil
	}
	return "", nil, ErrNoClipboardTool
}

// run executes the clipboard command, writing content to its stdin.
func run(ctx context.Context, name string, args []string, stdin io.Reader) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = stdin
	return cmd.Run()
}

// CommandCopier is a test seam: callers inject Cmd, Args, and Run.
// It does not perform any OS resolution, making it fully controllable
// in tests without real PATH probing or clipboard access.
type CommandCopier struct {
	Cmd  string
	Args []string
	Run  func(ctx context.Context, name string, args []string, stdin io.Reader) error
}

// Copy implements Copier by delegating to the injected Run function.
func (c CommandCopier) Copy(ctx context.Context, content []byte) error {
	return c.Run(ctx, c.Cmd, c.Args, bytes.NewReader(content))
}
