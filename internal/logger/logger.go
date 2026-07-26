package logger

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
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

// InputPrompt displays a colored input prompt.
func InputPrompt(message string) string {
	input, _ := InputPromptWithError(message)
	return input
}

// InputPromptWithError displays an input prompt and reports unavailable terminal input.
func InputPromptWithError(message string) (string, error) {
	if quietMode {
		return "", fmt.Errorf("interactive input is disabled in quiet mode")
	}
	fmt.Print(colorize(White+Bold, message))
	var input string
	_, errScan := fmt.Scanln(&input)
	if errScan == io.EOF {
		return "", errScan
	}
	return strings.TrimSpace(input), nil
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
	taskLogs   []LogMessage
	lastHeight int
	startTime  time.Time
	retryCount int
	status     string
	isRunning  bool
	managed    bool
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

// SetTotal sets the total number of work items.
func (pb *ProgressBar) SetTotal(total int) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.total = total
}

// SetStatus sets the task status text.
func (pb *ProgressBar) SetStatus(status string) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.status = terminalLine(status)
	pb.addTaskLogLocked(pb.status, Cyan)
}

// Complete marks the task as completed.
func (pb *ProgressBar) Complete() {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	if pb.total > 0 {
		pb.current = pb.total
	}
	pb.isLoading = false
	pb.isThinking = false
	pb.isSending = false
	pb.status = "Completed"
	pb.addTaskLogLocked(pb.status, Green)
}

// Fail marks the task as failed.
func (pb *ProgressBar) Fail(err error) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.isLoading = false
	pb.isThinking = false
	pb.isSending = false
	pb.status = terminalLine(fmt.Sprintf("Failed: %v", err))
	pb.addTaskLogLocked(pb.status, Red)
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

// PrintErrorAbove prints an error message above the progress bar and recreates the bar below
func (pb *ProgressBar) PrintErrorAbove(message, color string) {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	if quietMode {
		return
	}
	if pb.managed {
		line := terminalLine(message)
		pb.messages = append(pb.messages, colorize(color, line))
		pb.status = line
		pb.addTaskLogLocked(line, color)
		return
	}

	// Clear the current progress bar display
	if pb.lastHeight > 0 {
		// Move cursor up to the beginning of previous progress bar
		fmt.Printf("\033[%dA", pb.lastHeight)
		// Clear from cursor to end of screen
		fmt.Print("\033[J")
	}

	// Print the error message (this becomes part of the terminal history)
	fmt.Println(colorize(color, message))

	// Store the message for logging
	storeMessage(message, color)

	// Reset the progress bar state so it will render fresh below the error
	pb.lastHeight = 0
}

// AddMessage adds a message to display below the progress bar
func (pb *ProgressBar) AddMessage(message, color string) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	if pb.managed {
		message = terminalLine(message)
		pb.status = message
		pb.addTaskLogLocked(message, color)
	}
	coloredMessage := colorize(color, message)
	pb.messages = append(pb.messages, coloredMessage)
}

func (pb *ProgressBar) addTaskLogLocked(message, color string) {
	if !pb.managed || message == "" {
		return
	}
	pb.taskLogs = append(pb.taskLogs, LogMessage{
		Message:   message,
		Color:     color,
		Timestamp: time.Now(),
	})
}

// SaveTaskLogsToFile writes only messages generated by this progress task.
func (pb *ProgressBar) SaveTaskLogsToFile(filePath string) error {
	pb.mu.Lock()
	messages := append([]LogMessage(nil), pb.taskLogs...)
	pb.mu.Unlock()

	file, errCreate := os.Create(filePath)
	if errCreate != nil {
		return errCreate
	}
	for _, message := range messages {
		if _, errWrite := fmt.Fprintf(file, "[%s] %s\n", message.Timestamp.Format("2006-01-02 15:04:05"), message.Message); errWrite != nil {
			errClose := file.Close()
			if errClose != nil {
				return fmt.Errorf("failed to write task log: %v; failed to close task log: %w", errWrite, errClose)
			}
			return errWrite
		}
	}
	return file.Close()
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
	if pb.managed {
		return
	}
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

// MultiProgress renders multiple task bars from one refresh goroutine.
type MultiProgress struct {
	bars            []*ProgressBar
	writer          io.Writer
	refreshInterval time.Duration
	stopChan        chan struct{}
	doneChan        chan struct{}
	startOnce       sync.Once
	stopOnce        sync.Once
	mu              sync.Mutex
	lastHeight      int
	frame           int
	dynamic         bool
	terminalFD      int
	terminalWidth   int
}

// NewMultiProgress creates a centralized multi-task progress renderer.
func NewMultiProgress() *MultiProgress {
	terminalWidth, _, errTerminalSize := term.GetSize(int(os.Stdout.Fd()))
	if errTerminalSize != nil {
		terminalWidth = 0
	}
	multiProgress := newMultiProgress(os.Stdout, 500*time.Millisecond)
	multiProgress.dynamic = term.IsTerminal(int(os.Stdout.Fd()))
	multiProgress.terminalFD = int(os.Stdout.Fd())
	multiProgress.terminalWidth = terminalWidth
	return multiProgress
}

func newMultiProgress(writer io.Writer, refreshInterval time.Duration) *MultiProgress {
	return &MultiProgress{
		writer:          writer,
		refreshInterval: refreshInterval,
		stopChan:        make(chan struct{}),
		doneChan:        make(chan struct{}),
		dynamic:         true,
		terminalFD:      -1,
	}
}

// AddBar adds one file row to the centralized renderer.
func (mp *MultiProgress) AddBar(filename string) *ProgressBar {
	progressBar := &ProgressBar{
		barLength: 30,
		prefix:    terminalLine(filename),
		status:    "Waiting",
		startTime: time.Now(),
		isRunning: true,
		managed:   true,
	}
	mp.mu.Lock()
	mp.bars = append(mp.bars, progressBar)
	mp.mu.Unlock()
	return progressBar
}

// Start starts the single refresh goroutine.
func (mp *MultiProgress) Start() {
	mp.startOnce.Do(func() {
		if !mp.dynamic {
			close(mp.doneChan)
			return
		}
		mp.render()
		go mp.autoRender()
	})
}

// Stop waits for the renderer and leaves the final task states on screen.
func (mp *MultiProgress) Stop() {
	mp.Start()
	mp.stopOnce.Do(func() {
		if mp.dynamic {
			close(mp.stopChan)
			<-mp.doneChan
		}
		mp.render()
	})
}

func (mp *MultiProgress) autoRender() {
	ticker := time.NewTicker(mp.refreshInterval)
	defer ticker.Stop()
	defer close(mp.doneChan)

	for {
		select {
		case <-ticker.C:
			mp.render()
		case <-mp.stopChan:
			return
		}
	}
}

type progressBarSnapshot struct {
	current    int
	total      int
	barLength  int
	prefix     string
	suffix     string
	status     string
	isLoading  bool
	isThinking bool
	isSending  bool
	chunkSize  int
	retryCount int
	startTime  time.Time
	messages   []string
}

func (pb *ProgressBar) snapshot() progressBarSnapshot {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	return progressBarSnapshot{
		current:    pb.current,
		total:      pb.total,
		barLength:  pb.barLength,
		prefix:     pb.prefix,
		suffix:     pb.suffix,
		status:     pb.status,
		isLoading:  pb.isLoading,
		isThinking: pb.isThinking,
		isSending:  pb.isSending,
		chunkSize:  pb.chunkSize,
		retryCount: pb.retryCount,
		startTime:  pb.startTime,
		messages:   append([]string(nil), pb.messages...),
	}
}

func (mp *MultiProgress) render() {
	if quietMode {
		return
	}
	if mp.dynamic && mp.terminalFD >= 0 {
		if terminalWidth, _, errTerminalSize := term.GetSize(mp.terminalFD); errTerminalSize == nil {
			mp.terminalWidth = terminalWidth
		}
	}

	mp.mu.Lock()
	bars := append([]*ProgressBar(nil), mp.bars...)
	mp.mu.Unlock()
	if len(bars) == 0 {
		return
	}

	lines := make([]string, 0, len(bars)*3-1)
	for barIndex, progressBar := range bars {
		snapshot := progressBar.snapshot()
		lines = append(lines, wrapTerminalLine(snapshot.prefix, mp.terminalWidth)...)
		lines = append(lines, mp.renderProgressLine(snapshot))
		if barIndex < len(bars)-1 {
			lines = append(lines, "")
		}
	}

	if mp.dynamic && mp.lastHeight > 0 {
		_, _ = fmt.Fprintf(mp.writer, "\033[%dA\033[J", mp.lastHeight)
	}
	for _, line := range lines {
		_, _ = fmt.Fprintln(mp.writer, line)
	}
	mp.lastHeight = len(lines)
	mp.frame++
}

func terminalLine(value string) string {
	replacer := strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ")
	return strings.TrimSpace(replacer.Replace(value))
}

func (mp *MultiProgress) renderProgressLine(snapshot progressBarSnapshot) string {
	current := snapshot.current + snapshot.chunkSize
	progress := 0.0
	countText := fmt.Sprintf("%d/?", current)
	if snapshot.total > 0 {
		progress = float64(current) / float64(snapshot.total)
		if progress > 1 {
			progress = 1
		}
		countText = fmt.Sprintf("%d/%d", min(current, snapshot.total), snapshot.total)
	}
	if progress < 0 {
		progress = 0
	}

	barLength := snapshot.barLength
	status := snapshot.status
	if mp.terminalWidth > 0 {
		barLength = max(8, min(barLength, mp.terminalWidth/4))
		status = truncateTerminalLine(status, max(10, min(36, mp.terminalWidth/3)))
	}
	filledLength := int(float64(barLength) * progress)
	filledBar := strings.Repeat("█", filledLength)
	remainingBar := strings.Repeat("░", barLength-filledLength)
	bar := filledBar + remainingBar

	animation := loadingBars[mp.frame%len(loadingBars)]
	switch {
	case snapshot.isThinking:
		status = "Thinking " + animation
	case snapshot.isLoading:
		status = "Processing " + animation
	case snapshot.isSending && current < snapshot.total:
		status = "Sending batch ↑↑↑"
	case status == "":
		status = "Running"
	}

	elapsed := time.Since(snapshot.startTime)
	runtimeText := fmt.Sprintf("%02d:%02d:%02d", int(elapsed.Hours()), int(elapsed.Minutes())%60, int(elapsed.Seconds())%60)
	line := fmt.Sprintf("Runtime: %s |%s| %d%% (%s) | %s", runtimeText, bar, int(progress*100), countText, status)
	if snapshot.retryCount > 0 {
		line += fmt.Sprintf(" | Retries: %d", snapshot.retryCount)
	}
	if snapshot.suffix != "" {
		line += " | " + snapshot.suffix
	}
	if mp.terminalWidth > 1 {
		line = truncateTerminalLine(line, mp.terminalWidth-1)
	}

	styledBar := ""
	if filledBar != "" {
		styledBar += colorize(Green, filledBar)
	}
	if remainingBar != "" {
		styledBar += colorize(Blue, remainingBar)
	}
	line = strings.Replace(line, "|"+bar+"|", "|"+styledBar+"|", 1)
	line = strings.Replace(line, "Runtime: "+runtimeText, "Runtime: "+colorize(Cyan, runtimeText), 1)
	if snapshot.retryCount > 0 {
		retryCount := fmt.Sprintf("%d", snapshot.retryCount)
		line = strings.Replace(line, " | Retries: "+retryCount, " | Retries: "+colorize(Yellow, retryCount), 1)
	}
	return line
}

func wrapTerminalLine(value string, terminalWidth int) []string {
	if terminalWidth <= 1 || runewidth.StringWidth(value) < terminalWidth {
		return []string{value}
	}
	return strings.Split(runewidth.Wrap(value, terminalWidth-1), "\n")
}

func truncateTerminalLine(value string, maxWidth int) string {
	if maxWidth <= 0 || runewidth.StringWidth(value) <= maxWidth {
		return value
	}
	return runewidth.Truncate(value, maxWidth, "…")
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
