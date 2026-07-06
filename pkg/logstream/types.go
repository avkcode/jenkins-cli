package logstream

import (
	"io"
	"strings"
	"time"
)

const HistoryWait = 500 * time.Millisecond

type Record struct {
	Timestamp time.Time `json:"timestamp" yaml:"timestamp"`
	Source    string    `json:"source" yaml:"source"`
	Subject   string    `json:"subject,omitempty" yaml:"subject,omitempty"`
	Stream    string    `json:"stream,omitempty" yaml:"stream,omitempty"`
	Job       string    `json:"job,omitempty" yaml:"job,omitempty"`
	Build     int64     `json:"build,omitempty" yaml:"build,omitempty"`
	Node      string    `json:"node,omitempty" yaml:"node,omitempty"`
	VM        string    `json:"vm,omitempty" yaml:"vm,omitempty"`
	Stage     string    `json:"stage,omitempty" yaml:"stage,omitempty"`
	Line      string    `json:"line" yaml:"line"`
	Sequence  uint64    `json:"sequence,omitempty" yaml:"sequence,omitempty"`
}

type CursorState struct {
	Version   int               `json:"version" yaml:"version"`
	Timestamp time.Time         `json:"timestamp" yaml:"timestamp"`
	Streams   map[string]uint64 `json:"streams,omitempty" yaml:"streams,omitempty"`
	Sequences map[string]uint64 `json:"sequences,omitempty" yaml:"sequences,omitempty"`
}

func (c *CursorState) ensureMaps() {
	if c.Streams == nil {
		c.Streams = map[string]uint64{}
	}
	if c.Sequences == nil {
		c.Sequences = map[string]uint64{}
	}
}

type RenderOptions struct {
	Job          string
	Build        int64
	Source       string
	Prefix       bool
	Tail         int
	CursorPath   string
	Cursor       CursorState
	CursorLoaded bool
	Since        time.Time
	Until        time.Time
	Grep         string
	Level        string
	Stage        string
	Node         string
	VM           string
	Subject      string
	Format       string
	Err          io.Writer
}

type Renderer struct {
	out            io.Writer
	err            io.Writer
	opts           RenderOptions
	grep           matcher
	stageBySubject map[string]string
	linePending    []byte
	tailBuffer     []Record
	cursor         CursorState
	cursorWarned   bool
}

type matcher interface {
	MatchString(string) bool
}

func errorWriter(w io.Writer) io.Writer {
	if w != nil {
		return w
	}
	return io.Discard
}

func InferStreamName(subject string) string {
	if subject == "" {
		return ""
	}
	parts := strings.SplitN(subject, ".", 3)
	if len(parts) >= 2 {
		return strings.ToUpper(parts[0]) + "_" + strings.ToUpper(parts[1])
	}
	return strings.ToUpper(subject)
}

func SaveCursor(path string, cs CursorState) error {
	// Cursor saving is only useful with NATS streaming.
	// For HTTP-based Jenkins log streaming, cursors are not supported.
	return nil
}
