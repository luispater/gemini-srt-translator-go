package logger

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"
)

func TestSetColorMode(t *testing.T) {
	// Save original state
	originalUseColors := useColors

	// Test enabling colors
	SetColorMode(true)
	// The actual value depends on terminal support, so we just ensure it doesn't panic

	// Test disabling colors
	SetColorMode(false)
	if useColors {
		t.Error("Expected useColors to be false when SetColorMode(false) is called")
	}

	// Restore original state
	useColors = originalUseColors
}

func TestSetQuietMode(t *testing.T) {
	// Save original state
	originalQuiet := quietMode

	// Test enabling quiet mode
	SetQuietMode(true)
	if !quietMode {
		t.Error("Expected quietMode to be true")
	}

	// Test disabling quiet mode
	SetQuietMode(false)
	if quietMode {
		t.Error("Expected quietMode to be false")
	}

	// Restore original state
	quietMode = originalQuiet
}

func TestSupportsColor(t *testing.T) {
	// Test NO_COLOR environment variable
	originalNoColor := os.Getenv("NO_COLOR")
	originalForceColor := os.Getenv("FORCE_COLOR")

	// Test NO_COLOR set
	_ = os.Setenv("NO_COLOR", "1")
	if supportsColor() {
		t.Error("Expected supportsColor to return false when NO_COLOR is set")
	}

	// Test FORCE_COLOR set
	_ = os.Unsetenv("NO_COLOR")
	_ = os.Setenv("FORCE_COLOR", "1")
	if !supportsColor() {
		t.Error("Expected supportsColor to return true when FORCE_COLOR is set")
	}

	// Restore original environment
	if originalNoColor == "" {
		_ = os.Unsetenv("NO_COLOR")
	} else {
		_ = os.Setenv("NO_COLOR", originalNoColor)
	}
	if originalForceColor == "" {
		_ = os.Unsetenv("FORCE_COLOR")
	} else {
		_ = os.Setenv("FORCE_COLOR", originalForceColor)
	}
}

func TestColorize(t *testing.T) {
	// Save original state
	originalUseColors := useColors

	tests := []struct {
		name      string
		useColors bool
		color     string
		text      string
		want      string
	}{
		{
			name:      "with colors enabled",
			useColors: true,
			color:     Red,
			text:      "error message",
			want:      Red + "error message" + Reset,
		},
		{
			name:      "with colors disabled",
			useColors: false,
			color:     Red,
			text:      "error message",
			want:      "error message",
		},
		{
			name:      "empty text with colors",
			useColors: true,
			color:     Blue,
			text:      "",
			want:      Blue + "" + Reset,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useColors = tt.useColors
			got := colorize(tt.color, tt.text)
			if got != tt.want {
				t.Errorf("colorize() = %q, want %q", got, tt.want)
			}
		})
	}

	// Restore original state
	useColors = originalUseColors
}

func TestStoreMessage(t *testing.T) {
	// Clear existing messages
	logMutex.Lock()
	originalMessages := logMessages
	logMessages = nil
	logMutex.Unlock()

	// Store a test message
	testMessage := "test message"
	testColor := Red
	startTime := time.Now()
	storeMessage(testMessage, testColor)

	// Check that message was stored
	messages := GetStoredMessages()
	if len(messages) != 1 {
		t.Errorf("Expected 1 stored message, got %d", len(messages))
	}

	if len(messages) > 0 {
		msg := messages[0]
		if msg.Message != testMessage {
			t.Errorf("Expected message %q, got %q", testMessage, msg.Message)
		}
		if msg.Color != testColor {
			t.Errorf("Expected color %q, got %q", testColor, msg.Color)
		}
		if msg.Timestamp.Before(startTime) {
			t.Error("Expected timestamp to be after test start time")
		}
	}

	// Restore original messages
	logMutex.Lock()
	logMessages = originalMessages
	logMutex.Unlock()
}

func TestGetStoredMessages(t *testing.T) {
	// Clear existing messages
	logMutex.Lock()
	originalMessages := logMessages
	logMessages = []LogMessage{
		{Message: "msg1", Color: Red, Timestamp: time.Now()},
		{Message: "msg2", Color: Blue, Timestamp: time.Now()},
	}
	logMutex.Unlock()

	messages := GetStoredMessages()
	if len(messages) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(messages))
	}

	// Verify it's a copy (modifying returned slice shouldn't affect original)
	if len(messages) > 0 {
		messages[0].Message = "modified"
		om := GetStoredMessages()
		if om[0].Message == "modified" {
			t.Error("GetStoredMessages should return a copy, not the original slice")
		}
	}

	// Restore original messages
	logMutex.Lock()
	logMessages = originalMessages
	logMutex.Unlock()
}

func TestLogMessage(t *testing.T) {
	msg := LogMessage{
		Message:   "test message",
		Color:     Green,
		Timestamp: time.Now(),
	}

	if msg.Message != "test message" {
		t.Errorf("Expected message to be 'test message', got %q", msg.Message)
	}
	if msg.Color != Green {
		t.Errorf("Expected color to be Green, got %q", msg.Color)
	}
	if msg.Timestamp.IsZero() {
		t.Error("Expected timestamp to be set")
	}
}

func TestNewProgressBar(t *testing.T) {
	total := 100
	prefix := "Testing:"

	pb := NewProgressBar(total, prefix)
	defer pb.Stop()

	if pb.total != total {
		t.Errorf("Expected total to be %d, got %d", total, pb.total)
	}
	if pb.prefix != prefix {
		t.Errorf("Expected prefix to be %q, got %q", prefix, pb.prefix)
	}
	if pb.barLength != 30 {
		t.Errorf("Expected bar length to be 30, got %d", pb.barLength)
	}
	if pb.current != 0 {
		t.Errorf("Expected current to be 0, got %d", pb.current)
	}
}

func TestProgressBar_Update(t *testing.T) {
	pb := NewProgressBar(100, "Test")

	// Set quiet mode to avoid output during test
	originalQuiet := quietMode
	quietMode = true
	defer func() {
		pb.Stop()
		quietMode = originalQuiet
	}()

	pb.Update(50)
	if pb.current != 50 {
		t.Errorf("Expected current to be 50, got %d", pb.current)
	}

	pb.Update(100)
	if pb.current != 100 {
		t.Errorf("Expected current to be 100, got %d", pb.current)
	}
}

func TestProgressBar_SetSuffix(t *testing.T) {
	pb := NewProgressBar(100, "Test")

	// Set quiet mode to avoid output during test
	originalQuiet := quietMode
	quietMode = true
	defer func() {
		pb.Stop()
		quietMode = originalQuiet
	}()

	testSuffix := "model-name"
	pb.SetSuffix(testSuffix)
	if pb.suffix != testSuffix {
		t.Errorf("Expected suffix to be %q, got %q", testSuffix, pb.suffix)
	}
}

func TestProgressBar_SetLoading(t *testing.T) {
	pb := NewProgressBar(100, "Test")

	// Set quiet mode to avoid output during test
	originalQuiet := quietMode
	quietMode = true
	defer func() {
		pb.Stop()
		quietMode = originalQuiet
	}()

	pb.SetLoading(true)
	if !pb.isLoading {
		t.Error("Expected isLoading to be true")
	}

	pb.SetLoading(false)
	if pb.isLoading {
		t.Error("Expected isLoading to be false")
	}
}

func TestProgressBar_SetThinking(t *testing.T) {
	pb := NewProgressBar(100, "Test")

	// Set quiet mode to avoid output during test
	originalQuiet := quietMode
	quietMode = true
	defer func() {
		pb.Stop()
		quietMode = originalQuiet
	}()

	pb.SetThinking(true)
	if !pb.isThinking {
		t.Error("Expected isThinking to be true")
	}

	pb.SetThinking(false)
	if pb.isThinking {
		t.Error("Expected isThinking to be false")
	}
}

func TestProgressBar_SetSending(t *testing.T) {
	pb := NewProgressBar(100, "Test")

	// Set quiet mode to avoid output during test
	originalQuiet := quietMode
	quietMode = true
	defer func() {
		pb.Stop()
		quietMode = originalQuiet
	}()

	pb.SetSending(true)
	if !pb.isSending {
		t.Error("Expected isSending to be true")
	}

	pb.SetSending(false)
	if pb.isSending {
		t.Error("Expected isSending to be false")
	}
}

func TestProgressBar_AddMessage(t *testing.T) {
	pb := NewProgressBar(100, "Test")

	// Set quiet mode to avoid output during test
	originalQuiet := quietMode
	quietMode = true
	defer func() {
		pb.Stop()
		quietMode = originalQuiet
	}()

	testMessage := "API Key switched"
	testColor := Cyan

	pb.AddMessage(testMessage, testColor)
	if len(pb.messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(pb.messages))
	}

	// The message should be colorized
	expectedMessage := colorize(testColor, testMessage)
	if pb.messages[0] != expectedMessage {
		t.Errorf("Expected message %q, got %q", expectedMessage, pb.messages[0])
	}
}

func TestProgressBar_ThreadSafety(t *testing.T) {
	pb := NewProgressBar(1000, "Test")

	// Set quiet mode to avoid output during test
	originalQuiet := quietMode
	quietMode = true
	defer func() {
		pb.Stop()
		quietMode = originalQuiet
	}()

	// Test concurrent access
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			pb.Update(val * 10)
			pb.SetSuffix(fmt.Sprintf("suffix-%d", val))
			pb.SetLoading(val%2 == 0)
			pb.AddMessage(fmt.Sprintf("message-%d", val), Cyan)
		}(i)
	}

	wg.Wait()
	// If we get here without panicking, the test passes
}

func TestMultiProgressRendersTaskRowsCentrally(t *testing.T) {
	originalQuiet := quietMode
	originalColors := useColors
	quietMode = false
	useColors = false
	defer func() {
		quietMode = originalQuiet
		useColors = originalColors
	}()

	var output bytes.Buffer
	multiProgress := newMultiProgress(&output, time.Hour)
	firstBar := multiProgress.AddBar("episode-01.mkv")
	secondBar := multiProgress.AddBar("episode-02.mkv")
	firstBar.SetTotal(10)
	secondBar.SetTotal(20)
	multiProgress.Start()

	firstBar.Update(10)
	firstBar.Complete()
	secondBar.Update(5)
	secondBar.Fail(fmt.Errorf("request failed\nwith details"))
	multiProgress.Stop()

	segments := strings.Split(output.String(), "\033[J")
	finalRender := segments[len(segments)-1]
	for _, expected := range []string{"episode-01.mkv", "Completed", "episode-02.mkv", "Failed: request failed with details"} {
		if !strings.Contains(finalRender, expected) {
			t.Errorf("Final render does not contain %q:\n%s", expected, finalRender)
		}
	}
	if lineCount := strings.Count(finalRender, "\n"); lineCount != 5 {
		t.Errorf("Final render contains %d lines, want 5:\n%s", lineCount, finalRender)
	}
	if !strings.Contains(finalRender, "episode-01.mkv\nRuntime:") {
		t.Errorf("Filename is not above its progress line:\n%s", finalRender)
	}
	if !strings.Contains(finalRender, "\n\nepisode-02.mkv\nRuntime:") {
		t.Errorf("Task progress entries are not separated by an empty line:\n%s", finalRender)
	}
}

func TestMultiProgressUsesComponentColors(t *testing.T) {
	originalColors := useColors
	useColors = true
	defer func() { useColors = originalColors }()

	multiProgress := newMultiProgress(&bytes.Buffer{}, time.Hour)
	line := multiProgress.renderProgressLine(progressBarSnapshot{
		current:   5,
		total:     10,
		barLength: 10,
		prefix:    "episode.mkv",
		suffix:    "grok-4.5",
		status:    "Translating",
		startTime: time.Now(),
	})

	if !strings.HasPrefix(line, "Runtime: "+Cyan) {
		t.Errorf("Runtime is not in the first column: %q", line)
	}
	if strings.HasPrefix(line, Blue) {
		t.Errorf("Entire progress line is blue: %q", line)
	}
	if !strings.Contains(line, "|"+Green) {
		t.Errorf("Completed progress is not green: %q", line)
	}
	if !strings.Contains(line, Blue) {
		t.Errorf("Remaining progress is not blue: %q", line)
	}
	if !strings.Contains(line, "Runtime: "+Cyan) {
		t.Errorf("Runtime value is not cyan: %q", line)
	}
	if !strings.HasSuffix(line, "grok-4.5") {
		t.Errorf("Model name did not retain the default terminal color: %q", line)
	}
}

func TestMultiProgressNonTTYPrintsOnlyFinalState(t *testing.T) {
	originalQuiet := quietMode
	originalColors := useColors
	quietMode = false
	useColors = false
	defer func() {
		quietMode = originalQuiet
		useColors = originalColors
	}()

	var output bytes.Buffer
	multiProgress := newMultiProgress(&output, time.Millisecond)
	multiProgress.dynamic = false
	progressBar := multiProgress.AddBar("episode.srt")
	progressBar.SetTotal(10)
	multiProgress.Start()
	if output.Len() != 0 {
		t.Errorf("Non-TTY renderer wrote intermediate output: %q", output.String())
	}
	progressBar.Complete()
	multiProgress.Stop()
	if strings.Contains(output.String(), "\033[") {
		t.Errorf("Non-TTY renderer wrote ANSI cursor controls: %q", output.String())
	}
	if lineCount := strings.Count(output.String(), "\n"); lineCount != 2 {
		t.Errorf("Non-TTY renderer wrote %d lines, want 2: %q", lineCount, output.String())
	}
}

func TestWrapTerminalLinePreservesFullFilename(t *testing.T) {
	filename := "House.of.the.Dragon.S02E08.非常长的完整文件名.mkv"
	wrappedLines := wrapTerminalLine(filename, 20)
	if len(wrappedLines) < 2 {
		t.Fatalf("Filename was not wrapped: %v", wrappedLines)
	}
	if joinedFilename := strings.Join(wrappedLines, ""); joinedFilename != filename {
		t.Errorf("Wrapped filename = %q, want %q", joinedFilename, filename)
	}
	for _, line := range wrappedLines {
		if runewidth.StringWidth(line) >= 20 {
			t.Errorf("Wrapped line is too wide: %q", line)
		}
	}
}

func TestProgressBarsSaveIndependentTaskLogs(t *testing.T) {
	multiProgress := newMultiProgress(&bytes.Buffer{}, time.Hour)
	firstBar := multiProgress.AddBar("first.srt")
	secondBar := multiProgress.AddBar("second.srt")
	firstBar.SetStatus("first task status")
	firstBar.Complete()
	secondBar.SetStatus("second task status")
	secondBar.Fail(fmt.Errorf("second task failure"))

	firstLogPath := filepath.Join(t.TempDir(), "first.log")
	secondLogPath := filepath.Join(t.TempDir(), "second.log")
	if errSaveFirst := firstBar.SaveTaskLogsToFile(firstLogPath); errSaveFirst != nil {
		t.Fatalf("Failed to save first task log: %v", errSaveFirst)
	}
	if errSaveSecond := secondBar.SaveTaskLogsToFile(secondLogPath); errSaveSecond != nil {
		t.Fatalf("Failed to save second task log: %v", errSaveSecond)
	}
	firstLog, errReadFirst := os.ReadFile(firstLogPath)
	if errReadFirst != nil {
		t.Fatalf("Failed to read first task log: %v", errReadFirst)
	}
	secondLog, errReadSecond := os.ReadFile(secondLogPath)
	if errReadSecond != nil {
		t.Fatalf("Failed to read second task log: %v", errReadSecond)
	}
	if !strings.Contains(string(firstLog), "first task status") || !strings.Contains(string(firstLog), "Completed") || strings.Contains(string(firstLog), "second task status") {
		t.Errorf("First task log is not isolated: %s", firstLog)
	}
	if !strings.Contains(string(secondLog), "second task status") || !strings.Contains(string(secondLog), "second task failure") || strings.Contains(string(secondLog), "first task status") {
		t.Errorf("Second task log is not isolated: %s", secondLog)
	}
}

func TestSaveLogsToFile(t *testing.T) {
	// Create a temporary file
	tempFile, err := os.CreateTemp("", "test_log_*.txt")
	if err != nil {
		t.Fatal(err)
	}
	_ = tempFile.Close()
	defer func() {
		_ = os.Remove(tempFile.Name())
	}()

	// Store some test messages
	logMutex.Lock()
	originalMessages := logMessages
	logMessages = []LogMessage{
		{Message: "Info message", Color: Cyan, Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)},
		{Message: "Error message", Color: Red, Timestamp: time.Date(2024, 1, 1, 12, 1, 0, 0, time.UTC)},
	}
	logMutex.Unlock()

	// Save logs to file
	err = SaveLogsToFile(tempFile.Name())
	if err != nil {
		t.Fatalf("SaveLogsToFile failed: %v", err)
	}

	// Read the file and verify content
	content, err := os.ReadFile(tempFile.Name())
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "Info message") {
		t.Error("Expected log file to contain 'Info message'")
	}
	if !strings.Contains(contentStr, "Error message") {
		t.Error("Expected log file to contain 'Error message'")
	}
	if !strings.Contains(contentStr, "2024-01-01 12:00:00") {
		t.Error("Expected log file to contain formatted timestamp")
	}

	// Restore original messages
	logMutex.Lock()
	logMessages = originalMessages
	logMutex.Unlock()
}

func TestSaveLogsToFile_FileCreationError(t *testing.T) {
	// Try to save to an invalid path
	err := SaveLogsToFile("/invalid/path/that/does/not/exist/log.txt")
	if err == nil {
		t.Error("Expected error when saving to invalid path")
	}
}

// Test constant values
func TestConstants(t *testing.T) {
	if Reset != "\033[0m" {
		t.Errorf("Expected Reset to be '\\033[0m', got %q", Reset)
	}
	if Red != "\033[31m" {
		t.Errorf("Expected Red to be '\\033[31m', got %q", Red)
	}
	if Green != "\033[32m" {
		t.Errorf("Expected Green to be '\\033[32m', got %q", Green)
	}
}

// Test logging functions in quiet mode
func TestLoggingFunctionsQuietMode(t *testing.T) {
	// Save original state
	originalQuiet := quietMode

	// Clear existing messages and enable quiet mode
	logMutex.Lock()
	originalMessages := logMessages
	logMessages = nil
	logMutex.Unlock()

	quietMode = true

	// Call logging functions - they should not store messages in quiet mode
	Info("test info")
	Warning("test warning")
	Error("test error")
	Success("test success")
	Highlight("test highlight")

	// Check that no messages were stored
	messages := GetStoredMessages()
	if len(messages) != 0 {
		t.Errorf("Expected 0 stored messages in quiet mode, got %d", len(messages))
	}

	// Restore original state
	quietMode = originalQuiet
	logMutex.Lock()
	logMessages = originalMessages
	logMutex.Unlock()
}

func TestLoggingFunctionsNormalMode(t *testing.T) {
	// Save original state
	originalQuiet := quietMode

	// Clear existing messages
	logMutex.Lock()
	originalMessages := logMessages
	logMessages = nil
	logMutex.Unlock()

	quietMode = false

	// Call logging functions
	Info("test info")
	Warning("test warning")
	Error("test error")
	Success("test success")
	Highlight("test highlight")

	// Check that messages were stored
	messages := GetStoredMessages()
	if len(messages) != 5 {
		t.Errorf("Expected 5 stored messages, got %d", len(messages))
	}

	expectedMessages := []struct {
		text  string
		color string
	}{
		{"test info", Cyan},
		{"test warning", Yellow},
		{"test error", Red},
		{"test success", Green},
		{"test highlight", Magenta},
	}

	for i, expected := range expectedMessages {
		if i < len(messages) {
			if messages[i].Message != expected.text {
				t.Errorf("Expected message %d to be %q, got %q", i, expected.text, messages[i].Message)
			}
			if messages[i].Color != expected.color {
				t.Errorf("Expected message %d color to be %q, got %q", i, expected.color, messages[i].Color)
			}
		}
	}

	// Restore original state
	quietMode = originalQuiet
	logMutex.Lock()
	logMessages = originalMessages
	logMutex.Unlock()
}
