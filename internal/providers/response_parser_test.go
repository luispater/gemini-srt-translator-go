package providers

import "testing"

func TestParseTranslatedBatchRepairsMissingContentKey(t *testing.T) {
	responseText := `[
  {"index": 256, "content": "打扰一下"},
  {"index": 257, "- 嗯哼    - 你好"},
  {"index": 258 "- 她很有脾气    - 几个星期内"},
  {"index": 259, "- She's got spirit.\r\n- Couple weeks," "content": "- 她很有脾气    - 几个星期内"},
  {"index": 260="- 也带过来了    - 那些夜晚"},
  {"index": 261 "- 哎，我会跟他们说的。你保重。    - 好的。","guard":"GST_LINE_000261"},
  {"index": 262:// content has colon
  "content":"- 关于我的问题    - 你在说什么啊","guard":"GST_LINE_000262"},
  {"index": 263:// Spanish subtitle
  "不，他根本不在那里    - 罗伯-威尔在哪里    ","guard":"GST_LINE_000263"},
  {"index": 264, "content": "我想安排我的六头阉牛进行屠宰"}
]`

	translatedBatch, repairedResponseText, err := parseTranslatedBatch(responseText)
	if err != nil {
		t.Fatalf("Expected repaired response to parse, got error: %v", err)
	}
	if len(translatedBatch) != 9 {
		t.Fatalf("Expected 9 translated objects, got %d", len(translatedBatch))
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
	if translatedBatch[5].Index != 261 {
		t.Errorf("Expected repaired object index 261, got %d", translatedBatch[5].Index)
	}
	if translatedBatch[5].Content != "- 哎，我会跟他们说的。你保重。    - 好的。" {
		t.Errorf("Expected repaired content before guard to be preserved, got %q", translatedBatch[5].Content)
	}
	if translatedBatch[5].Guard != "GST_LINE_000261" {
		t.Errorf("Expected repaired guard to be preserved, got %q", translatedBatch[5].Guard)
	}
	if translatedBatch[6].Index != 262 {
		t.Errorf("Expected repaired object index 262, got %d", translatedBatch[6].Index)
	}
	if translatedBatch[6].Content != "- 关于我的问题    - 你在说什么啊" {
		t.Errorf("Expected repaired content after index comment to be preserved, got %q", translatedBatch[6].Content)
	}
	if translatedBatch[6].Guard != "GST_LINE_000262" {
		t.Errorf("Expected repaired guard after index comment to be preserved, got %q", translatedBatch[6].Guard)
	}
	if translatedBatch[7].Index != 263 {
		t.Errorf("Expected repaired object index 263, got %d", translatedBatch[7].Index)
	}
	if translatedBatch[7].Content != "不，他根本不在那里    - 罗伯-威尔在哪里    " {
		t.Errorf("Expected repaired content value after index comment to be preserved, got %q", translatedBatch[7].Content)
	}
	if translatedBatch[7].Guard != "GST_LINE_000263" {
		t.Errorf("Expected repaired guard after index comment value to be preserved, got %q", translatedBatch[7].Guard)
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
