package logging_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/cosmosfactory/recetas-backend/internal/platform/logging"
)

func decodeLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	raw := strings.TrimRight(buf.String(), "\n")
	if raw == "" {
		return nil
	}
	lines := strings.Split(raw, "\n")
	out := make([]map[string]any, 0, len(lines))
	for i, line := range lines {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("line %d is not a JSON object: %v (line=%q)", i, err, line)
		}
		out = append(out, rec)
	}
	return out
}

func TestNew_WritesOneJSONObjectPerLine(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := logging.New(&buf, slog.LevelDebug)

	logger.Info("server started", "addr", ":8080")
	logger.Error("query failed", "err", "boom", "attempt", 2)

	if !strings.HasSuffix(buf.String(), "\n") {
		t.Fatalf("got output %q, want it to end with a newline", buf.String())
	}
	recs := decodeLines(t, &buf)
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2 (output=%q)", len(recs), buf.String())
	}

	tests := []struct {
		name  string
		rec   map[string]any
		level string
		msg   string
		attrs map[string]any
	}{
		{name: "info", rec: recs[0], level: "INFO", msg: "server started", attrs: map[string]any{"addr": ":8080"}},
		{name: "error", rec: recs[1], level: "ERROR", msg: "query failed", attrs: map[string]any{"err": "boom", "attempt": float64(2)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range []string{"time", "level", "msg"} {
				if _, ok := tt.rec[k]; !ok {
					t.Errorf("got record %v, want key %q", tt.rec, k)
				}
			}
			if got := tt.rec["level"]; got != tt.level {
				t.Errorf("got level %v, want %q", got, tt.level)
			}
			if got := tt.rec["msg"]; got != tt.msg {
				t.Errorf("got msg %v, want %q", got, tt.msg)
			}
			for k, want := range tt.attrs {
				if got := tt.rec[k]; got != want {
					t.Errorf("got attr %s=%v, want %v", k, got, want)
				}
			}
		})
	}
}

func TestNew_DropsRecordsBelowLevel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		level      slog.Level
		wantLevels []string
	}{
		{name: "debug keeps all", level: slog.LevelDebug, wantLevels: []string{"DEBUG", "INFO", "WARN", "ERROR"}},
		{name: "info drops debug", level: slog.LevelInfo, wantLevels: []string{"INFO", "WARN", "ERROR"}},
		{name: "warn drops info", level: slog.LevelWarn, wantLevels: []string{"WARN", "ERROR"}},
		{name: "error keeps only error", level: slog.LevelError, wantLevels: []string{"ERROR"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			logger := logging.New(&buf, tt.level)

			logger.Debug("d")
			logger.Info("i")
			logger.Warn("w")
			logger.Error("e")

			recs := decodeLines(t, &buf)
			if len(recs) != len(tt.wantLevels) {
				t.Fatalf("got %d records, want %d (output=%q)", len(recs), len(tt.wantLevels), buf.String())
			}
			for i, want := range tt.wantLevels {
				if got := recs[i]["level"]; got != want {
					t.Errorf("record %d: got level %v, want %q", i, got, want)
				}
			}
		})
	}
}
