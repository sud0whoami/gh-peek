package domain

import "testing"

func TestStartContextKindValues(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		got  StartContextKind
		want string
	}{
		{"pr", StartContextPR, "pr"},
		{"all", StartContextAll, "all"},
		{"branch", StartContextBranch, "branch"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.got) != tc.want {
				t.Fatalf("StartContextKind %s = %q, want %q", tc.name, string(tc.got), tc.want)
			}
		})
	}
}
