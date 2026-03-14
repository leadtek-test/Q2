package logging_test

import (
	"errors"
	"testing"

	"q2/pkg/logging"
)

func TestParseLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    logging.Level
		wantErr error
	}{
		{name: "debug", input: "DEBUG", want: logging.LevelDebug},
		{name: "warning alias", input: "warning", want: logging.LevelWarn},
		{name: "all empty", input: "", want: logging.LevelAll},
		{name: "invalid", input: "TRACE", wantErr: logging.ErrInvalidLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := logging.ParseLevel(tt.input)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ParseLevel(%q) error = %v, want %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
