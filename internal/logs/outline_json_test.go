package logs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sud0whoami/gh-peek/internal/testutil"
)

func TestOutlineToJSON_Nil(t *testing.T) {
	result := OutlineToJSON(nil, nil)
	if result != nil {
		t.Errorf("OutlineToJSON(nil, nil) = %v, want nil", result)
	}
}

func TestOutlineToJSON_EmptyBuffer(t *testing.T) {
	b := New()
	o := BuildOutline(b)
	result := OutlineToJSON(o, b)
	// Empty buffer → empty outline → empty (but non-nil) result.
	if result == nil {
		t.Error("OutlineToJSON with empty buffer returned nil, want non-nil slice")
	}
}

func TestOutlineToJSON_NodeKinds(t *testing.T) {
	raw := "2024-01-01T00:00:00Z ##[group]My Step\n" +
		"2024-01-01T00:00:00Z ##[group]inner group\n" +
		"2024-01-01T00:00:00Z ##[error]an error\n" +
		"2024-01-01T00:00:00Z ##[endgroup]\n" +
		"2024-01-01T00:00:00Z ##[endgroup]\n"
	b := New()
	b.Set([]byte(raw))
	o := BuildOutline(b)
	nodes := OutlineToJSON(o, b)

	if len(nodes) == 0 {
		t.Fatal("want at least one root node")
	}

	root := nodes[0]
	if root.Kind != "step" {
		t.Errorf("root.Kind = %q, want %q", root.Kind, "step")
	}
	if root.Title != "My Step" {
		t.Errorf("root.Title = %q, want %q", root.Title, "My Step")
	}
	if root.ErrorCount != 1 {
		t.Errorf("root.ErrorCount = %d, want 1", root.ErrorCount)
	}
	if root.Severity != "error" {
		t.Errorf("root.Severity = %q, want %q", root.Severity, "error")
	}

	// Find inner group child.
	var groupNode *NodeJSON
	for _, c := range root.Children {
		if c.Kind == "group" {
			groupNode = c
			break
		}
	}
	if groupNode == nil {
		t.Fatal("want a group child under the step")
	}
	if groupNode.Title != "inner group" {
		t.Errorf("groupNode.Title = %q, want %q", groupNode.Title, "inner group")
	}

	// Find line child under the group.
	var lineNode *NodeJSON
	for _, c := range groupNode.Children {
		if c.Kind == "line" {
			lineNode = c
			break
		}
	}
	if lineNode == nil {
		t.Fatal("want a line child under the group")
	}
	if lineNode.Severity != "error" {
		t.Errorf("lineNode.Severity = %q, want %q", lineNode.Severity, "error")
	}
	if lineNode.Text == "" {
		t.Error("lineNode.Text should not be empty")
	}
	if lineNode.LineNumber <= 0 {
		t.Errorf("lineNode.LineNumber = %d, want > 0", lineNode.LineNumber)
	}
	// NodeLine should have no children.
	if lineNode.Children != nil {
		t.Errorf("lineNode.Children = %v, want nil", lineNode.Children)
	}
}

func TestOutlineToJSON_SeverityNames(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"plain line", "none"},
		{"##[debug]debug msg", "debug"},
		{"##[command]run cmd", "command"},
		{"##[notice]note", "notice"},
		{"##[warning]warn", "warning"},
		{"##[error]err", "error"},
	}

	for _, tc := range cases {
		raw := "2024-01-01T00:00:00Z ##[group]step\n2024-01-01T00:00:00Z " + tc.raw + "\n2024-01-01T00:00:00Z ##[endgroup]\n"
		b := New()
		b.Set([]byte(raw))
		o := BuildOutline(b)
		nodes := OutlineToJSON(o, b)
		if len(nodes) == 0 || len(nodes[0].Children) == 0 {
			t.Errorf("case %q: want at least one child node", tc.raw)
			continue
		}
		child := nodes[0].Children[0]
		if child.Severity != tc.want {
			t.Errorf("case %q: severity = %q, want %q", tc.raw, child.Severity, tc.want)
		}
	}
}

func TestOutlineToJSON_TextIsANSIStripped(t *testing.T) {
	raw := "2024-01-01T00:00:00Z ##[group]step\n" +
		"2024-01-01T00:00:00Z \x1b[31m##[error]red error\x1b[0m\n" +
		"2024-01-01T00:00:00Z ##[endgroup]\n"
	b := New()
	b.Set([]byte(raw))
	o := BuildOutline(b)
	nodes := OutlineToJSON(o, b)

	var findLine func([]*NodeJSON) *NodeJSON
	findLine = func(ns []*NodeJSON) *NodeJSON {
		for _, n := range ns {
			if n.Kind == "line" {
				return n
			}
			if found := findLine(n.Children); found != nil {
				return found
			}
		}
		return nil
	}

	lineNode := findLine(nodes)
	if lineNode == nil {
		t.Fatal("no line node found")
	}
	for _, b := range []byte(lineNode.Text) {
		if b == 0x1b {
			t.Errorf("line text contains ANSI escape: %q", lineNode.Text)
			break
		}
	}
}

func loadJSONTestFixture(t *testing.T, name string) *Buffer {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("load fixture %q: %v", name, err)
	}
	b := New()
	b.Set(data)
	return b
}

func TestOutlineToJSON_Golden(t *testing.T) {
	b := loadJSONTestFixture(t, "failing.log")
	o := BuildOutline(b)
	nodes := OutlineToJSON(o, b)

	data, err := json.MarshalIndent(nodes, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Normalize timestamps before golden comparison.
	actual := testutil.NormalizeTimestamps(string(data))
	testutil.AssertGolden(t, "failing.outline.json", actual)
}
