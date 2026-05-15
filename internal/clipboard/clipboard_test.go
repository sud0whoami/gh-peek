package clipboard_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/sud0whoami/gh-peek/internal/clipboard"
)

// mockLookPath returns a lookPath function that succeeds only for the
// provided set of command names (values are the resolved paths).
func mockLookPath(available map[string]string) func(string) (string, error) {
	return func(name string) (string, error) {
		if p, ok := available[name]; ok {
			return p, nil
		}
		return "", errors.New("not found")
	}
}

func TestPickLinuxCommand_WaylandPreferred(t *testing.T) {
	look := mockLookPath(map[string]string{
		"wl-copy": "/usr/bin/wl-copy",
		"xclip":   "/usr/bin/xclip",
	})
	getenv := func(key string) string {
		if key == "WAYLAND_DISPLAY" {
			return "wayland-0"
		}
		return ""
	}

	cmd, args, err := clipboard.PickLinuxCommand(look, getenv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != "/usr/bin/wl-copy" {
		t.Errorf("cmd = %q, want %q", cmd, "/usr/bin/wl-copy")
	}
	if len(args) != 0 {
		t.Errorf("args = %v, want empty", args)
	}
}

func TestPickLinuxCommand_XclipFallback(t *testing.T) {
	look := mockLookPath(map[string]string{
		"xclip": "/usr/bin/xclip",
	})
	getenv := func(string) string { return "" }

	cmd, args, err := clipboard.PickLinuxCommand(look, getenv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != "/usr/bin/xclip" {
		t.Errorf("cmd = %q, want %q", cmd, "/usr/bin/xclip")
	}
	want := []string{"-selection", "clipboard"}
	if len(args) != len(want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}

func TestPickLinuxCommand_XselFallback(t *testing.T) {
	look := mockLookPath(map[string]string{
		"xsel": "/usr/bin/xsel",
	})
	getenv := func(string) string { return "" }

	cmd, args, err := clipboard.PickLinuxCommand(look, getenv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != "/usr/bin/xsel" {
		t.Errorf("cmd = %q, want %q", cmd, "/usr/bin/xsel")
	}
	want := []string{"--clipboard", "--input"}
	if len(args) != len(want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}

func TestPickLinuxCommand_NoToolReturnsError(t *testing.T) {
	look := mockLookPath(map[string]string{})
	getenv := func(string) string { return "" }

	_, _, err := clipboard.PickLinuxCommand(look, getenv)
	if !errors.Is(err, clipboard.ErrNoClipboardTool) {
		t.Errorf("err = %v, want ErrNoClipboardTool", err)
	}
}

func TestCommandCopier_PassesContentViaStdin(t *testing.T) {
	want := []byte("hello clipboard")
	var got []byte

	c := clipboard.CommandCopier{
		Cmd:  "testcmd",
		Args: []string{"--flag"},
		Run: func(_ context.Context, name string, args []string, stdin io.Reader) error {
			var buf bytes.Buffer
			if _, err := io.Copy(&buf, stdin); err != nil {
				return err
			}
			got = buf.Bytes()
			return nil
		},
	}

	if err := c.Copy(context.Background(), want); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("stdin content = %q, want %q", got, want)
	}
}

func TestCommandCopier_PropagatesError(t *testing.T) {
	boom := errors.New("boom")
	c := clipboard.CommandCopier{
		Cmd: "testcmd",
		Run: func(_ context.Context, _ string, _ []string, _ io.Reader) error {
			return boom
		},
	}

	err := c.Copy(context.Background(), []byte("data"))
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}
