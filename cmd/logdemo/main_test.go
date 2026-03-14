package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogDemo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		args  []string
		wants []string
	}{
		{
			name:  "memory",
			args:  []string{"run", "./cmd/logdemo", "-backend=memory"},
			wants: []string{"== all persisted logs ==", "== remaining after clear ==", "handler processed 3 entries", " INFO service booted"},
		},
		{
			name:  "file",
			args:  []string{"run", "./cmd/logdemo", "-backend=file", "-file=" + filepath.Join(t.TempDir(), "demo.jsonl")},
			wants: []string{"== all persisted logs ==", "== remaining after clear ==", "handler processed 3 entries", " INFO service booted"},
		},
		{
			name:  "json format",
			args:  []string{"run", "./cmd/logdemo", "-backend=memory", "-format=json"},
			wants: []string{"== all persisted logs ==", "== remaining after clear ==", "handler processed 3 entries", "\"message\":\"service booted\""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command("go", tt.args...)
			cmd.Dir = "../.."

			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("go %s error = %v\noutput:\n%s", strings.Join(tt.args, " "), err, out)
			}

			output := string(out)
			for _, want := range tt.wants {
				if !strings.Contains(output, want) {
					t.Fatalf("output missing %q:\n%s", want, output)
				}
			}
		})
	}
}
