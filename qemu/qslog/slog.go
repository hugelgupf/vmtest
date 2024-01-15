// Package qslog implements an event channel for slog JSON data.
package qslog

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hugelgupf/vmtest/qemu"
)

// Record is an slog record as transmitted via the JSON handler.
type Record struct {
	Time  time.Time
	Msg   string
	Level slog.Level
	Attrs map[string]any
}

// String formats a record just like slog's default handler would (or as close
// as possible).
func (s Record) String() string {
	var b strings.Builder
	b.WriteString(s.Time.Format(time.RFC3339))
	b.WriteString(" ")
	b.WriteString(s.Level.String())
	b.WriteString(" ")
	b.WriteString(s.Msg)
	b.WriteString(" ")
	writeAttrs(&b, "", s.Attrs)
	return b.String()
}

func writeAttrs(b *strings.Builder, prefix string, attrs map[string]any) {
	for key, value := range attrs {
		switch v := value.(type) {
		case map[string]interface{}:
			var np string
			if prefix == "" {
				np = key
			} else {
				np = prefix + "." + key
			}
			writeAttrs(b, np, v)

		default:
			if prefix != "" {
				b.WriteString(prefix)
				b.WriteString(".")
			}
			b.WriteString(key)
			b.WriteString("=")
			b.WriteString(fmt.Sprintf("%v", v))
			b.WriteString(" ")
		}
	}
}

// RecordFrom converts an arbitrary JSON-decoded slog record.
func RecordFrom(r map[string]any) Record {
	var nr Record
	if st, ok := r["time"].(string); ok {
		t, _ := time.Parse(time.RFC3339Nano, st)
		nr.Time = t
		delete(r, "time")
	}
	if msg, ok := r["msg"].(string); ok {
		nr.Msg = msg
		delete(r, "msg")
	}
	if level, ok := r["level"].(string); ok {
		_ = nr.Level.UnmarshalText([]byte(level))
		delete(r, "level")
	}
	nr.Attrs = r
	return nr
}

// ReadEventFile reads a vmtest event channel file full of slog records.
func ReadEventFile(path string) ([]Record, error) {
	errors, err := qemu.ReadEventFile[map[string]any](path)
	if err != nil {
		return nil, err
	}
	var records []Record
	for _, r := range errors {
		records = append(records, RecordFrom(r))
	}
	return records, nil
}
