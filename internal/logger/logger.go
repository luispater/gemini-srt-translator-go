package logger

import (
	"fmt"
	"golang.org/x/term"
	"os"
	"strings"
	"sync"
	"time"
)

// ANSI color codes
const (
	Reset   = "\033[0m"
	Bold    = "\033[1m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	White   = "\033[37m"
)

var (
	useColors    = true
	quietMode    = false
	logMessages  []LogMessage
	logMutex     sync.RWMutex
	loadingBars  = []string{"—", "\\", "|", "/"}
	loadingIndex = 0
)

// LogMessage represents a stored log message
type LogMessage struct {
	Message   string
	Color     string
	Timestamp time.Time
}

// SetColorMode enables or disables color output
func SetColorMode(enabled bool) {
	useColors = enabled && supportsColor()
}

// SetQuietMode enables or disables quiet mode
func SetQuietMode(enabled bool) {
	quietMode = enabled
}

// supportsColor checks if the terminal supports color output
func supportsColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("FORCE_COLOR") != "" {
		return true
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// colorize applies color to text if colors are enabled
func colorize(color, text string) string {
	if useColors {
		return color + text + Reset
	}
	return text
}

// Info prints an information message in cyan
func Info(message string) {
	if quietMode {
		return
	}
	fmt.Println(colorize(Cyan, message))
	storeMessage(message, Cyan)
}

// Warning prints a warning message in yellow
func Warning(message string) {
	if quietMode {
		return
	}
	fmt.Println(colorize(Yellow, message))
	storeMessage(message, Yellow)
}

// Error prints an error message in red
func Error(message string) {
	if quietMode {
		return
	}
	fmt.Println(colorize(Red, message))
	storeMessage(message, Red)
}

// Success prints a success message in green
func Success(message string) {
	if quietMode {
		return
	}
	fmt.Println(colorize(Green, message))
	storeMessage(message, Green)
}

// Highlight prints an important message in bold magenta
func Highlight(message string) {
	if quietMode {
		return
	}
	fmt.Println(colorize(Magenta+Bold, message))
	storeMessage(message, Magenta)
}

// InputPrompt displays a colored input prompt
func InputPrompt(message string) string {
	if quietMode {
		return ""
	}
	fmt.Print(colorize(White+Bold, message))
	var input string
	_, _ = fmt.Scanln(&input)
	return strings.TrimSpace(input)
}

// storeMessage stores a log message for later retrieval
func storeMessage(message, color string) {
	logMutex.Lock()
	defer logMutex.Unlock()
	logMessages = append(logMessages, LogMessage{
		Message:   message,
		Color:     color,
		Timestamp: time.Now(),
	})
}

// GetStoredMessages returns all stored log messages
func GetStoredMessages() []LogMessage {
	logMutex.RLock()
	defer logMutex.RUnlock()
	messages := make([]LogMessage, len(logMessages))
	copy(messages, logMessages)
	return messages
}

// ProgressBar represents a progress bar
type ProgressBar struct {
	current    int
	total      int
	barLength  int
	prefix     string
	suffix     string
	isLoading  bool
	isThinking bool
	isSending  bool
	chunkSize  int
	messages   []string
	lastHeight int
	startTime  time.Time
	retryCount int
	isRunning  bool
	stopChan   chan bool
	mu         sync.Mutex
}

// NewProgressBar creates a new progress bar
func NewProgressBar(total int, prefix string) *ProgressBar {
	pb := &ProgressBar{
		total:     total,
		barLength: 30,
		prefix:    prefix,
		startTime: time.Now(),
		isRunning: true,
		stopChan:  make(chan bool, 1),
	}

	// Start auto-rendering goroutine
	go pb.autoRender()

	return pb
}

// Update updates the progress bar
func (pb *ProgressBar) Update(current int) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.current = current
}

// SetSuffix sets the suffix text
func (pb *ProgressBar) SetSuffix(suffix string) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.suffix = suffix
}

// SetLoading sets loading animation state
func (pb *ProgressBar) SetLoading(loading bool) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.isLoading = loading
}

// SetThinking sets thinking animation state
func (pb *ProgressBar) SetThinking(thinking bool) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.isThinking = thinking
}

// SetSending sets sending state
func (pb *ProgressBar) SetSending(sending bool) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.isSending = sending
}

// AddMessage adds a message to display below the progress bar
func (pb *ProgressBar) AddMessage(message, color string) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	coloredMessage := colorize(color, message)
	pb.messages = append(pb.messages, coloredMessage)
}

// AddRetry increments the retry count
func (pb *ProgressBar) AddRetry() {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.retryCount++
}

// Stop stops the auto-rendering goroutine
func (pb *ProgressBar) Stop() {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	if pb.isRunning {
		// Render one final time to show the complete state
		pb.renderInternal()

		pb.isRunning = false
		select {
		case pb.stopChan <- true:
		default:
		}
		close(pb.stopChan)

		// Give a brief moment for the final render to be fully displayed
		pb.mu.Unlock()
		time.Sleep(100 * time.Millisecond)
		pb.mu.Lock()
	}
}

// autoRender runs in a goroutine and renders the progress bar every 0.5 seconds
func (pb *ProgressBar) autoRender() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pb.mu.Lock()
			if !pb.isRunning {
				pb.mu.Unlock()
				return
			}
			pb.renderInternal()
			pb.mu.Unlock()
		case <-pb.stopChan:
			return
		}
	}
}

// renderInternal renders the progress bar (called by autoRender goroutine)
func (pb *ProgressBar) renderInternal() {
	if quietMode {
		return
	}

	// Calculate progress
	progress := float64(pb.current+pb.chunkSize) / float64(pb.total)
	if progress > 1.0 {
		progress = 1.0
	}

	filledLength := int(float64(pb.barLength) * progress)
	bar := strings.Repeat("█", filledLength) + strings.Repeat("░", pb.barLength-filledLength)
	percentage := int(progress * 100)

	// Calculate runtime
	elapsed := time.Since(pb.startTime)
	hours := int(elapsed.Hours())
	minutes := int(elapsed.Minutes()) % 60
	seconds := int(elapsed.Seconds()) % 60
	runtimeStr := fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)

	// Build progress text with runtime and retry count
	progressText := fmt.Sprintf("%s |%s| %d%% (%d/%d) | Runtime: %s",
		pb.prefix,
		colorize(Green, bar),
		percentage,
		pb.current+pb.chunkSize,
		pb.total,
		colorize(Cyan, runtimeStr),
	)

	// Add retry count if there were retries
	if pb.retryCount > 0 {
		progressText += fmt.Sprintf(" | Retries: %s", colorize(Yellow, fmt.Sprintf("%d", pb.retryCount)))
	}

	if pb.suffix != "" {
		progressText += " ｜ " + pb.suffix
	}

	// Add animation
	if pb.isThinking {
		progressText += " | Thinking " + colorize(Green, loadingBars[loadingIndex%len(loadingBars)])
		loadingIndex++
	} else if pb.isLoading {
		progressText += " | Processing " + colorize(Green, loadingBars[loadingIndex%len(loadingBars)])
		loadingIndex++
	} else if pb.isSending && pb.current < pb.total {
		progressText += " | Sending batch " + colorize(Green, "↑↑↑")
	}

	// Clear previous output if exists
	if pb.lastHeight > 0 {
		// Move cursor up to the beginning of previous progress bar
		fmt.Printf("\033[%dA", pb.lastHeight)
		// Clear from cursor to end of screen
		fmt.Print("\033[J")
	}

	// Calculate current height (progress bar + empty line + messages)
	currentHeight := 2 + len(pb.messages)

	// Print the progress bar
	fmt.Println(colorize(Blue, progressText))
	fmt.Println() // Empty line for separation

	// Print messages
	for _, msg := range pb.messages {
		fmt.Println(msg)
	}

	// Store current height for next render
	pb.lastHeight = currentHeight
}

// SaveLogsToFile saves all stored messages to a file
func SaveLogsToFile(filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer func() {
		if errClose := file.Close(); errClose != nil {
			Error(fmt.Sprintf("Error closing log file: %v", errClose))
		}
	}()

	messages := GetStoredMessages()
	for _, msg := range messages {
		_, errWrite := fmt.Fprintf(file, "[%s] %s\n",
			msg.Timestamp.Format("2006-01-02 15:04:05"),
			msg.Message)
		if errWrite != nil {
			return errWrite
		}
	}

	return nil
}
