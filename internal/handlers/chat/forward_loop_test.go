package chat

import (
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type loopingSSEReader struct {
	chunks int
}

func (l *loopingSSEReader) Read(p []byte) (int, error) {
	if l.chunks >= 60 {
		return 0, io.EOF
	}
	var line string
	if l.chunks < 15 {
		line = fmt.Sprintf(`data: {"choices":[{"delta":{"content":"intro context %d words ahead here. "}}]}`+"\n\n", l.chunks)
	} else {
		line = `data: {"choices":[{"delta":{"content":"alpha beta gamma delta epsilon zeta "}}]}` + "\n\n"
	}
	l.chunks++
	n := copy(p, line)
	return n, nil
}

func TestHandleStreamResponse_AbortsOnRepetitionLoop(t *testing.T) {
	reader := &loopingSSEReader{}
	rec := httptest.NewRecorder()
	h := &ChatHandler{}
	metrics := &streamMetrics{}

	if err := h.handleStreamResponse(context.Background(), rec, reader, false, time.Now(), metrics); err != nil {
		t.Fatalf("handleStreamResponse returned error: %v", err)
	}
	if reader.chunks >= 60 {
		t.Fatalf("expected early abort before EOF, read %d chunks", reader.chunks)
	}
	out := rec.Body.String()
	if !strings.Contains(out, `"type":"loop_detected"`) {
		t.Fatalf("expected loop_detected SSE frame, got: %s", out)
	}
	if !strings.Contains(out, "data: [DONE]") {
		t.Fatalf("expected terminal [DONE] marker, got: %s", out)
	}
}

