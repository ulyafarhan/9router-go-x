package proxy

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// A trailing block of p words repeated 3+ times inside a long output.
func loopedText(blockWords int, repeats int, prefix string) string {
	block := make([]string, blockWords)
	for i := range block {
		block[i] = fmt.Sprintf("w%d", i)
	}
	blockStr := strings.Join(block, " ")
	parts := []string{prefix}
	for i := 0; i < repeats; i++ {
		parts = append(parts, blockStr)
	}
	return strings.Join(parts, " ")
}

func TestDetectChatCompletionLoopOpenAI(t *testing.T) {
	text := loopedText(6, 12, strings.Repeat("intro sentence filler here. ", 12))
	body, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{{
			"message": map[string]any{"role": "assistant", "content": text},
		}},
	})
	looped, detail := DetectChatCompletionLoop(body)
	if !looped {
		t.Fatalf("looped text not detected")
	}
	if detail == "" {
		t.Fatal("empty loop detail")
	}
}

func TestDetectChatCompletionLoopClaudeBlocks(t *testing.T) {
	text := loopedText(4, 16, strings.Repeat("steady prose filler words here. ", 12))
	body, _ := json.Marshal(map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
	})
	looped, _ := DetectChatCompletionLoop(body)
	if !looped {
		t.Fatal("claude content-array loop not detected")
	}
}

func TestDetectChatCompletionLoopNormalText(t *testing.T) {
	// Long coherent text: unique sentences, only natural short repetition.
	var sb strings.Builder
	for i := 0; i < 120; i++ {
		fmt.Fprintf(&sb, "Sentence number %d discusses topic %d with its own distinct wording. ", i, i%7)
	}
	body, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{{"message": map[string]any{"content": sb.String()}}},
	})
	if looped, _ := DetectChatCompletionLoop(body); looped {
		t.Fatal("normal varied text flagged as a loop")
	}
}

func TestDetectChatCompletionLoopShortRepetitionArmsLate(t *testing.T) {
	// A 5-word block repeated 3 times is only 15 words: below the arming
	// threshold, so legitimate short repetition (chorus lines, lists) passes.
	text := strings.Repeat("la la la la la ", 3) + strings.Repeat("normal tail content follows here. ", 10)
	body, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{{"message": map[string]any{"content": text}}},
	})
	if looped, _ := DetectChatCompletionLoop(body); looped {
		t.Fatal("short repetition flagged; detector armed too early")
	}
}

func TestDetectChatCompletionLoopGarbageBody(t *testing.T) {
	for _, body := range []string{"", "not json", "[]", `{"choices":[]}`, `{"choices":[{"message":{"content":42}}]}`} {
		if looped, _ := DetectChatCompletionLoop([]byte(body)); looped {
			t.Fatalf("garbage body %q flagged as a loop", body)
		}
	}
}

func TestStreamLoopDetectorIncrementalOpenAIAndClaude(t *testing.T) {
	d := NewStreamLoopDetector()
	intro := strings.Repeat("steady intro context sentence here. ", 12)
	// Feed word fragments across chunk boundaries.
	chunkSize := 7
	for i := 0; i < len(intro); i += chunkSize {
		end := min(i+chunkSize, len(intro))
		if looped, _ := d.PushText(intro[i:end]); looped {
			t.Fatalf("intro triggered premature loop")
		}
	}
	loopBlock := "alpha beta gamma delta epsilon zeta "
	triggered := false
	for i := 0; i < 14; i++ {
		payload, _ := json.Marshal(map[string]any{
			"choices": []map[string]any{{"delta": map[string]any{"content": loopBlock}}},
		})
		if looped, detail := d.PushSSEPayload(payload); looped {
			if !strings.Contains(detail, "6-word block") {
				t.Fatalf("unexpected detail: %q", detail)
			}
			triggered = true
			break
		}
	}
	if !triggered {
		t.Fatal("streaming OpenAI loop was never detected")
	}

	// Claude text_delta format
	cd := NewStreamLoopDetector()
	for i := 0; i < 30; i++ {
		payload, _ := json.Marshal(map[string]any{
			"type":  "content_block_delta",
			"delta": map[string]any{"type": "text_delta", "text": "prefix variation words before loop. "},
		})
		_, _ = cd.PushSSEPayload(payload)
	}
	claudeLooped := false
	for i := 0; i < 16; i++ {
		payload, _ := json.Marshal(map[string]any{
			"type":  "content_block_delta",
			"delta": map[string]any{"type": "text_delta", "text": "one two three four "},
		})
		if looped, _ := cd.PushSSEPayload(payload); looped {
			claudeLooped = true
			break
		}
	}
	if !claudeLooped {
		t.Fatal("streaming Claude loop was never detected")
	}
}
