package proxy

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The loop probe is part of the gateway's auto-tuner: a completion whose text
// repeats itself is a failure the routing layer can act on (the combo moves to
// the next member), not garbage the client reads. Detection is word-level and
// deliberately conservative: it only fires on long outputs where the same
// word-block repeats back to back at least three times, so legitimate lists,
// chorus lines, and code with repeated statements are never touched.
const (
	// loopMinWords arms the detector: shorter outputs cannot carry a
	// destructive loop, and cutting them risks trimming legitimate repetition.
	loopMinWords = 90
	// loopMinSpanWords is the smallest total repeated span (repeats × period)
	// that counts as a loop: at three repeats, a 20-word period.
	loopMinSpanWords = 60
	// loopMaxPeriod bounds the word-block length the detector scans for.
	loopMaxPeriod = 32
	// loopMinPeriod skips trivial 1-word stutters, which models emit for
	// emphasis without being broken.
	loopMinPeriod = 2
	// loopRepeatThreshold is how many consecutive identical blocks make a loop.
	loopRepeatThreshold = 3
	// loopMaxScanWords caps how much text the detector reads per call.
	loopMaxScanWords = 5000
)

// DetectChatCompletionLoop reports whether a non-streaming chat completion
// body (OpenAI `choices[].message.content`, or Claude `content[].text`) is a
// repetition loop, and names the loop shape for the log.
func DetectChatCompletionLoop(body []byte) (bool, string) {
	text := extractCompletionText(body)
	if text == "" {
		return false, ""
	}
	period, repeats, ok := detectWordRepetition(text)
	if !ok {
		return false, ""
	}
	return true, fmt.Sprintf("same %d-word block repeated %d times back to back", period, repeats)
}

// extractCompletionText pulls the assistant text out of an OpenAI
// chat.completion body or a Claude Messages body. Unparseable or non-text
// content yields "" so the probe never guesses.
func extractCompletionText(body []byte) string {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return ""
	}
	var probe struct {
		Choices jsontext.Value `json:"choices"`
		Content jsontext.Value `json:"content"`
	}
	if err := json.Unmarshal(trimmed, &probe); err != nil {
		return ""
	}

	// OpenAI chat format: choices[0].message.content
	if probe.Choices != nil {
		var choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(probe.Choices, &choices); err == nil && len(choices) > 0 {
			return contentText(choices[0].Message.Content)
		}
		return ""
	}

	// Claude format: content is a string or an array of {type,text} blocks.
	if probe.Content != nil {
		return contentText(json.RawMessage(probe.Content))
	}
	return ""
}

// contentText accepts a JSON string or an array of {type:"text",text:...}
// blocks and returns the plain text they carry.
func contentText(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	var s string
	if err := json.Unmarshal(trimmed, &s); err == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(trimmed, &blocks); err == nil {
		var b strings.Builder
		for _, blk := range blocks {
			if blk.Type == "text" || blk.Type == "" {
				b.WriteString(blk.Text)
				b.WriteString("\n")
			}
		}
		return strings.TrimSpace(b.String())
	}
	return ""
}

// detectWordRepetition finds the shortest trailing word-block that repeats
// back to back, subject to the minimum repeat count and span thresholds.
func detectWordRepetition(text string) (period, repeats int, ok bool) {
	words := strings.Fields(text)
	if len(words) > loopMaxScanWords {
		words = words[len(words)-loopMaxScanWords:]
	}
	return findWordLoop(words)
}

func findWordLoop(words []string) (period, repeats int, ok bool) {
	if len(words) < loopMinWords {
		return 0, 0, false
	}
	maxPeriod := loopMaxPeriod
	if maxP := len(words) / loopRepeatThreshold; maxP < maxPeriod {
		maxPeriod = maxP
	}
	for p := loopMinPeriod; p <= maxPeriod; p++ {
		if p*loopRepeatThreshold > len(words) {
			break
		}
		// Count the consecutive repeats of the trailing p-word block by
		// walking one block backwards while each block equals the next.
		r := 1
		end := len(words)
		for end-2*p >= 0 && equalWords(words[end-2*p:end-p], words[end-p:end]) {
			r++
			end -= p
		}
		if r >= loopRepeatThreshold && p*r >= loopMinSpanWords {
			return p, r, true
		}
	}
	return 0, 0, false
}

// equalWords reports whether two word slices are identical.
func equalWords(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

const (
	// streamLoopWindowWords bounds memory per stream while holding enough
	// history for max period * 8 repeats (> loopMinWords=90).
	streamLoopWindowWords = 256
	// streamLoopMaxWordBytes bounds a single non-whitespace token (e.g. accidental base64).
	streamLoopMaxWordBytes = 256
)

// StreamLoopDetector watches incremental SSE text deltas in bounded memory and
// triggers as soon as the emitted word stream enters a destructive repetition loop.
type StreamLoopDetector struct {
	words   []string
	partial strings.Builder
}

// NewStreamLoopDetector returns a bounded streaming repetition detector.
func NewStreamLoopDetector() *StreamLoopDetector {
	return &StreamLoopDetector{words: make([]string, 0, streamLoopWindowWords)}
}

// PushText feeds one text delta into the rolling word window.
func (d *StreamLoopDetector) PushText(delta string) (bool, string) {
	if d == nil || delta == "" {
		return false, ""
	}
	added := false
	for _, r := range delta {
		if unicode.IsSpace(r) {
			if d.flushPartial() {
				added = true
			}
			continue
		}
		if d.partial.Len() < streamLoopMaxWordBytes {
			_, _ = d.partial.WriteRune(r)
		}
	}
	if !added || len(d.words) < loopMinWords {
		return false, ""
	}
	if p, rep, ok := findWordLoop(d.words); ok {
		return true, fmt.Sprintf("same %d-word block repeated %d times back to back", p, rep)
	}
	return false, ""
}

func (d *StreamLoopDetector) flushPartial() bool {
	if d.partial.Len() == 0 {
		return false
	}
	w := d.partial.String()
	d.partial.Reset()
	if len(d.words) >= streamLoopWindowWords*2 {
		// Compact in place so active streams stay bounded under 1-vCPU RAM caps.
		copy(d.words, d.words[len(d.words)-streamLoopWindowWords:])
		d.words = d.words[:streamLoopWindowWords]
	}
	d.words = append(d.words, w)
	return true
}

// PushSSEPayload extracts text deltas from OpenAI Chat, Claude Messages, or
// Responses SSE JSON payloads and feeds them to PushText.
func (d *StreamLoopDetector) PushSSEPayload(payload []byte) (bool, string) {
	if d == nil || len(payload) == 0 || bytes.Equal(bytes.TrimSpace(payload), []byte("[DONE]")) {
		return false, ""
	}
	// Fast skip for non-text chunks (usage, ping, tool-only frames).
	if !bytes.Contains(payload, []byte(`"content"`)) && !bytes.Contains(payload, []byte(`"text"`)) && !bytes.Contains(payload, []byte(`"delta"`)) {
		return false, ""
	}
	if delta := extractStreamTextDelta(payload); delta != "" {
		return d.PushText(delta)
	}
	return false, ""
}

func extractStreamTextDelta(payload []byte) string {
	var probe struct {
		Type    string `json:"type"`
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
		Delta json.RawMessage `json:"delta"`
	}
	if err := json.Unmarshal(payload, &probe); err != nil {
		return ""
	}
	if len(probe.Choices) > 0 && probe.Choices[0].Delta.Content != "" {
		return probe.Choices[0].Delta.Content
	}
	if len(probe.Delta) == 0 {
		return ""
	}
	if probe.Type == "response.output_text.delta" {
		var s string
		if err := json.Unmarshal(probe.Delta, &s); err == nil {
			return s
		}
	}
	var claudeDelta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(probe.Delta, &claudeDelta); err == nil && (claudeDelta.Type == "text_delta" || claudeDelta.Text != "") {
		return claudeDelta.Text
	}
	// OpenAI Chat streams use UTF-8 without trailing whitespace until the next token;
	// utf8.ValidString check is unnecessary beyond json.Unmarshal.
	_ = utf8.RuneCountInString
	return ""
}
