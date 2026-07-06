package logstream

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var stagePattern = regexp.MustCompile(`^\[Pipeline\]\s+\{\s+\(([^)]+)\)`)

func NewRenderer(out io.Writer, opts RenderOptions) (*Renderer, error) {
	if out == nil {
		out = io.Discard
	}
	opts.CursorPath = strings.TrimSpace(opts.CursorPath)
	opts.Level = strings.TrimSpace(opts.Level)
	opts.Stage = strings.TrimSpace(opts.Stage)
	opts.Node = strings.TrimSpace(opts.Node)
	opts.Grep = strings.TrimSpace(opts.Grep)
	opts.Format = strings.TrimSpace(opts.Format)
	opts.Err = errorWriter(opts.Err)

	var grep matcher
	if opts.Grep != "" {
		re, err := regexp.Compile(opts.Grep)
		if err != nil {
			return nil, fmt.Errorf("invalid --grep: %w", err)
		}
		grep = re
	}

	cursor := opts.Cursor
	cursor.ensureMaps()
	if cursor.Version == 0 {
		cursor.Version = 2
	}

	return &Renderer{
		out:            out,
		err:            opts.Err,
		opts:           opts,
		grep:           grep,
		stageBySubject: map[string]string{},
		cursor:         cursor,
	}, nil
}

func (r *Renderer) Write(p []byte) (int, error) {
	r.linePending = append(r.linePending, p...)
	for {
		idx := -1
		for i, b := range r.linePending {
			if b == '\n' {
				idx = i
				break
			}
		}
		if idx < 0 {
			break
		}
		line := strings.TrimRight(string(r.linePending[:idx]), "\r")
		r.linePending = append([]byte(nil), r.linePending[idx+1:]...)
		if err := r.Emit(Record{
			Timestamp: time.Now().UTC(),
			Source:    r.opts.Source,
			Subject:   r.opts.Subject,
			Job:       r.opts.Job,
			Build:     r.opts.Build,
			Node:      r.opts.Node,
			VM:        r.opts.VM,
			Line:      line,
		}); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (r *Renderer) Flush() error {
	if len(r.linePending) > 0 {
		line := strings.TrimRight(string(r.linePending), "\r")
		r.linePending = nil
		if err := r.Emit(Record{
			Timestamp: time.Now().UTC(),
			Source:    r.opts.Source,
			Subject:   r.opts.Subject,
			Job:       r.opts.Job,
			Build:     r.opts.Build,
			Node:      r.opts.Node,
			VM:        r.opts.VM,
			Line:      line,
		}); err != nil {
			return err
		}
	}
	return r.FlushTail()
}

func (r *Renderer) Emit(record Record) error {
	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now().UTC()
	}
	if record.Source == "" {
		record.Source = r.opts.Source
	}
	if record.Job == "" {
		record.Job = r.opts.Job
	}
	if record.Build == 0 {
		record.Build = r.opts.Build
	}
	if record.Stream == "" {
		record.Stream = InferStreamName(record.Subject)
	}
	if record.Stage == "" {
		record.Stage = r.stageForRecord(record)
	}
	if !r.match(record) {
		return nil
	}
	r.updateCursor(record)
	if r.opts.Tail > 0 {
		r.tailBuffer = append(r.tailBuffer, record)
		if len(r.tailBuffer) > r.opts.Tail {
			r.tailBuffer = append([]Record(nil), r.tailBuffer[len(r.tailBuffer)-r.opts.Tail:]...)
		}
		return nil
	}
	return r.render(record)
}

func (r *Renderer) FlushTail() error {
	if len(r.tailBuffer) == 0 {
		return nil
	}
	for _, record := range r.tailBuffer {
		if err := r.render(record); err != nil {
			return err
		}
	}
	r.tailBuffer = nil
	return nil
}

func (r *Renderer) DisableTail() {
	r.opts.Tail = 0
	r.tailBuffer = nil
}

func (r *Renderer) stageForRecord(record Record) string {
	key := record.Subject
	if key == "" {
		key = record.Source
	}
	if match := stagePattern.FindStringSubmatch(record.Line); len(match) == 2 {
		r.stageBySubject[key] = match[1]
		return match[1]
	}
	return r.stageBySubject[key]
}

func (r *Renderer) match(record Record) bool {
	if !r.opts.Since.IsZero() && record.Timestamp.Before(r.opts.Since) {
		return false
	}
	if !r.opts.Until.IsZero() && record.Timestamp.After(r.opts.Until) {
		return false
	}
	if r.grep != nil && !r.grep.MatchString(record.Line) {
		return false
	}
	if r.opts.Level != "" && !strings.Contains(strings.ToLower(record.Line), strings.ToLower(r.opts.Level)) {
		return false
	}
	if r.opts.Stage != "" && !strings.EqualFold(record.Stage, r.opts.Stage) {
		return false
	}
	if r.opts.Node != "" && !strings.EqualFold(record.Node, r.opts.Node) {
		return false
	}
	return true
}

func (r *Renderer) render(record Record) error {
	switch r.opts.Format {
	case "json":
		data, err := json.Marshal(record)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(r.out, string(data))
		return err
	case "yaml":
		data, err := yaml.Marshal(record)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(r.out, "---\n"); err != nil {
			return err
		}
		_, err = r.out.Write(data)
		return err
	default:
		prefix := ""
		if r.opts.Prefix {
			prefix = RecordPrefix(record)
		}
		_, err := fmt.Fprintln(r.out, prefix+record.Line)
		return err
	}
}

func RecordPrefix(record Record) string {
	switch record.Source {
	case "vm":
		if record.VM != "" {
			return "[vm:" + record.VM + "] "
		}
		return "[vm] "
	case "jenkins":
		if record.Node != "" {
			return "[jenkins:" + record.Node + "] "
		}
		return "[jenkins] "
	default:
		if record.Source != "" {
			return "[" + record.Source + "] "
		}
		return ""
	}
}

func (r *Renderer) updateCursor(record Record) {
	if r.opts.CursorPath == "" {
		return
	}
	if record.Timestamp.After(r.cursor.Timestamp) {
		r.cursor.Timestamp = record.Timestamp
	}
	stream := record.Stream
	if stream == "" {
		stream = InferStreamName(record.Subject)
	}
	if record.Sequence > 0 && stream != "" {
		prev := r.cursor.Streams[stream]
		if record.Sequence > prev {
			r.cursor.Streams[stream] = record.Sequence
		}
	}
	if record.Sequence > 0 && record.Subject != "" {
		if record.Sequence > r.cursor.Sequences[record.Subject] {
			r.cursor.Sequences[record.Subject] = record.Sequence
		}
	}
	if err := SaveCursor(r.opts.CursorPath, r.cursor); err != nil && !r.cursorWarned {
		r.cursorWarned = true
		fmt.Fprintf(r.err, "Warning: failed to save log cursor %s: %v\n", r.opts.CursorPath, err)
	}
}
