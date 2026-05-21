package providers

import "testing"

func TestParseTranslatedBatchRepairsMissingContentKey(t *testing.T) {
	responseText := `[
  {"index": 256, "content": "打扰一下"},
  {"index": 257, "- 嗯哼    - 你好"},
  {"index": 258 "- 她很有脾气    - 几个星期内"},
  {"index": 259, "- She's got spirit.\r\n- Couple weeks," "content": "- 她很有脾气    - 几个星期内"},
  {"index": 260="- 也带过来了    - 那些夜晚"},
  {"index": 261, "content": "我想安排我的六头阉牛进行屠宰"}
]`

	translatedBatch, repairedResponseText, err := parseTranslatedBatch(responseText)
	if err != nil {
		t.Fatalf("Expected repaired response to parse, got error: %v", err)
	}
	if len(translatedBatch) != 6 {
		t.Fatalf("Expected 6 translated objects, got %d", len(translatedBatch))
	}
	if translatedBatch[1].Index != 257 {
		t.Errorf("Expected repaired object index 257, got %d", translatedBatch[1].Index)
	}
	if translatedBatch[1].Content != "- 嗯哼    - 你好" {
		t.Errorf("Expected repaired content to be preserved, got %q", translatedBatch[1].Content)
	}
	if translatedBatch[2].Index != 258 {
		t.Errorf("Expected repaired object index 258, got %d", translatedBatch[2].Index)
	}
	if translatedBatch[2].Content != "- 她很有脾气    - 几个星期内" {
		t.Errorf("Expected repaired content without comma to be preserved, got %q", translatedBatch[2].Content)
	}
	if translatedBatch[3].Index != 259 {
		t.Errorf("Expected repaired object index 259, got %d", translatedBatch[3].Index)
	}
	if translatedBatch[3].Content != "- 她很有脾气    - 几个星期内" {
		t.Errorf("Expected repaired content with stray key to be preserved, got %q", translatedBatch[3].Content)
	}
	if translatedBatch[4].Index != 260 {
		t.Errorf("Expected repaired object index 260, got %d", translatedBatch[4].Index)
	}
	if translatedBatch[4].Content != "- 也带过来了    - 那些夜晚" {
		t.Errorf("Expected repaired content with equal sign to be preserved, got %q", translatedBatch[4].Content)
	}
	if repairedResponseText == responseText {
		t.Error("Expected repaired response text to differ from original")
	}
}

func TestRepairMissingContentKeysDoesNotChangeValidJSON(t *testing.T) {
	responseText := `[{"index":1,"content":"{\"index\":2,\"bad\"}"}]`

	repairedResponseText := repairMissingContentKeys(responseText)
	if repairedResponseText != responseText {
		t.Errorf("Expected valid response to remain unchanged, got %s", repairedResponseText)
	}
}

func TestParseTranslatedBatchKeepsFirstRepeatedArray(t *testing.T) {
	responseText := `[{"index":1,"content":"one"}][{"index":1,"content":"one"}]`

	translatedBatch, repairedResponseText, err := parseTranslatedBatch(responseText)
	if err != nil {
		t.Fatalf("Expected repeated array response to parse, got error: %v", err)
	}
	if len(translatedBatch) != 1 {
		t.Fatalf("Expected 1 translated object, got %d", len(translatedBatch))
	}
	if translatedBatch[0].Content != "one" {
		t.Errorf("Expected content to be preserved, got %q", translatedBatch[0].Content)
	}
	if repairedResponseText != `[{"index":1,"content":"one"}]` {
		t.Errorf("Expected first array only, got %s", repairedResponseText)
	}
}
