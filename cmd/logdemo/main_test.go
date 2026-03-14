package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLogDemo(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "demo.jsonl")
	clearPath := filepath.Join(t.TempDir(), "demo-clear.jsonl")
	multiPath := filepath.Join(t.TempDir(), "demo-multi.jsonl")

	tests := []struct {
		name           string
		args           []string
		wants          []string
		verifyFilePath string
		wantEmptyFile  bool
	}{
		{
			name:  "memory",
			args:  []string{"run", "./cmd/logdemo", "-backend=memory"},
			wants: []string{"== all persisted logs ==", "== remaining logs ==", "handler processed 3 entries", " INFO service booted"},
		},
		{
			name:           "file",
			args:           []string{"run", "./cmd/logdemo", "-backend=file", "-file=" + filePath},
			wants:          []string{"== all persisted logs ==", "== remaining logs ==", "handler processed 3 entries", " INFO service booted"},
			verifyFilePath: filePath,
		},
		{
			name:  "json format",
			args:  []string{"run", "./cmd/logdemo", "-backend=memory", "-format=json"},
			wants: []string{"== all persisted logs ==", "== remaining logs ==", "handler processed 3 entries", "\"message\":\"service booted\""},
		},
		{
			name:           "clear logs",
			args:           []string{"run", "./cmd/logdemo", "-backend=file", "-file=" + clearPath, "-clear=true"},
			wants:          []string{"== all persisted logs ==", "== remaining logs ==", "handler processed 3 entries"},
			verifyFilePath: clearPath,
			wantEmptyFile:  true,
		},
		{
			name:           "multi backend memory and file",
			args:           []string{"run", "./cmd/logdemo", "-backend=memory,file", "-file=" + multiPath},
			wants:          []string{"== all persisted logs ==", "== remaining logs ==", "handler processed 3 entries", " INFO service booted"},
			verifyFilePath: multiPath,
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

			if tt.verifyFilePath != "" {
				info, statErr := os.Stat(tt.verifyFilePath)
				if statErr != nil {
					t.Fatalf("stat %q error = %v", tt.verifyFilePath, statErr)
				}
				if tt.wantEmptyFile && info.Size() != 0 {
					t.Fatalf("file %q size = %d, want 0", tt.verifyFilePath, info.Size())
				}
				if !tt.wantEmptyFile && info.Size() == 0 {
					t.Fatalf("file %q should not be empty", tt.verifyFilePath)
				}
			}
		})
	}
}

func TestParseBackends(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "single", input: "file", want: []string{"file"}},
		{name: "multi", input: "memory,file,influx", want: []string{"memory", "file", "influx"}},
		{name: "trim and dedupe", input: " file ,memory,file ", want: []string{"file", "memory"}},
		{name: "empty", input: "  , ", want: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseBackends(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseBackends(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
