package ass

import (
	"bufio"
	stdErrors "errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const escapedTagPrefix = "[[\x00ASSTAG"

var (
	reASSTagPlaceholder = regexp.MustCompile(`\[\[ASSTAG(\d+)\]\]`)
)

func isAsciiLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isLibassSpace(c byte) bool {
	return c == ' ' || c == '\t'
}

func trimLibassSpaces(s string) string {
	start := 0
	for start < len(s) && isLibassSpace(s[start]) {
		start++
	}
	end := len(s)
	for end > start && isLibassSpace(s[end-1]) {
		end--
	}
	return s[start:end]
}

func isCSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}

func parseLeadingInt(s string) (int, bool) {
	i := 0
	for i < len(s) && isCSpace(s[i]) {
		i++
	}
	if i >= len(s) {
		return 0, false
	}
	isNegative := false
	startSign := i
	if s[i] == '-' {
		isNegative = true
		i++
	} else if s[i] == '+' {
		i++
	}
	startDigits := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == startDigits {
		return 0, false
	}
	val64, errParse := strconv.ParseInt(s[startSign:i], 10, 32)
	if errParse != nil {
		var numErr *strconv.NumError
		if stdErrors.As(errParse, &numErr) && stdErrors.Is(numErr.Err, strconv.ErrRange) {
			if isNegative {
				return math.MinInt32, true
			}
			return math.MaxInt32, true
		}
		return 0, false
	}
	return int(val64), true
}

type tagCommand struct {
	name   string
	val    int
	hasVal bool
}

func countNonEmptyArgs(s string) int {
	parts := strings.Split(s, ",")
	count := 0
	for _, p := range parts {
		if trimLibassSpaces(p) != "" {
			count++
		}
	}
	return count
}

func parseTagCommands(tag string) []tagCommand {
	if !strings.Contains(tag, "{") {
		return parseSingleTagBlockCommands(tag)
	}
	var commands []tagCommand
	i := 0
	for i < len(tag) {
		openIdx := strings.IndexByte(tag[i:], '{')
		if openIdx == -1 {
			break
		}
		openIdx += i
		closeIdx := strings.IndexByte(tag[openIdx:], '}')
		if closeIdx == -1 {
			break
		}
		closeIdx += openIdx
		blockContent := tag[openIdx+1 : closeIdx]
		i = closeIdx + 1

		commands = append(commands, parseSingleTagBlockCommands(blockContent)...)
	}
	return commands
}

func parseSingleTagBlockCommands(s string) []tagCommand {
	var commands []tagCommand
	i := 0
	inT := false

	for i < len(s) {
		c := s[i]
		if c == '}' {
			inT = false
			i++
			continue
		}
		if c == ')' {
			inT = false
			i++
			continue
		}
		if c != '\\' {
			i++
			continue
		}

		// Found '\'
		i++
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			break
		}

		if s[i] == '(' || s[i] == '\\' || s[i] == '}' || (s[i] == ')' && inT) {
			continue
		}

		rem := s[i:]
		cmdName := ""
		if strings.HasPrefix(rem, "pos") || strings.HasPrefix(rem, "pbo") {
			cmdName = rem[:3]
			i += 3
		} else if rem[0] == 'p' {
			cmdName = "p"
			i++
		} else if rem[0] == 'q' {
			cmdName = "q"
			i++
		} else if rem[0] == 't' {
			cmdName = "t"
			i++
		} else {
			for i < len(s) && s[i] != '(' && s[i] != '\\' && s[i] != '}' && (!inT || s[i] != ')') && s[i] != ' ' && s[i] != '\t' {
				i++
			}
		}

		if cmdName == "t" {
			for i < len(s) && s[i] != '(' && s[i] != '\\' && s[i] != '}' && (!inT || s[i] != ')') {
				i++
			}
			if i < len(s) && s[i] == '(' {
				i++ // skip '('
				startArgs := i
				firstBs := -1
				for i < len(s) {
					ch := s[i]
					if ch == '\\' {
						firstBs = i
						break
					} else if ch == ')' || ch == '}' {
						break
					}
					i++
				}
				prefix := ""
				if firstBs != -1 {
					prefix = s[startArgs:firstBs]
				} else {
					prefix = s[startArgs:i]
				}
				lastComma := strings.LastIndex(prefix, ",")
				valid := true
				if lastComma != -1 {
					if countNonEmptyArgs(prefix[:lastComma]) >= 4 {
						valid = false
					}
				}
				if !valid {
					for i < len(s) && s[i] != ')' && s[i] != '}' {
						i++
					}
					if i < len(s) && s[i] == ')' {
						i++
						inT = false
					}
					continue
				}
				inT = true
			}
			continue
		}

		startArg := i
		parenStart := -1
		parenEnd := -1
		inParen := false

		for i < len(s) {
			ch := s[i]
			if ch == '(' && !inParen {
				inParen = true
				parenStart = i - startArg
				i++
			} else if ch == ')' && inParen {
				inParen = false
				parenEnd = i - startArg
				i++
				inT = false
				break
			} else if ch == '}' {
				if inParen {
					parenEnd = i - startArg
					inParen = false
				}
				inT = false
				break
			} else if ch == '\\' && !inParen {
				break
			} else if ch == ')' && !inParen && inT {
				inT = false
				break
			} else {
				i++
			}
		}

		argStr := s[startArg:i]

		if cmdName == "p" || cmdName == "q" {
			val := 0
			hasVal := false
			if parenStart != -1 {
				endIdx := parenEnd
				if endIdx == -1 || endIdx > len(argStr) {
					endIdx = len(argStr)
				}
				inside := argStr[parenStart+1 : endIdx]
				parts := strings.Split(inside, ",")
				for _, part := range parts {
					part = trimLibassSpaces(part)
					if part != "" {
						v, ok := parseLeadingInt(part)
						if ok {
							val = v
						} else {
							val = 0
						}
						hasVal = true
						break
					}
				}
				if !hasVal {
					pfx := trimLibassSpaces(argStr[:parenStart])
					if pfx != "" {
						v, ok := parseLeadingInt(pfx)
						if ok {
							val = v
						} else {
							val = 0
						}
						hasVal = true
					}
				}
			} else {
				sTrimmed := trimLibassSpaces(argStr)
				if sTrimmed != "" {
					v, ok := parseLeadingInt(sTrimmed)
					if ok {
						val = v
					} else {
						val = 0
					}
					hasVal = true
				}
			}
			commands = append(commands, tagCommand{
				name:   cmdName,
				val:    val,
				hasVal: hasVal,
			})
		}
	}
	return commands
}

func parseTagDrawingMode(tag string) (drawingOn bool, hasCmd bool) {
	cmds := parseTagCommands(tag)
	for _, cmd := range cmds {
		if cmd.name == "p" {
			hasCmd = true
			drawingOn = cmd.hasVal && cmd.val > 0
		}
	}
	return drawingOn, hasCmd
}

func parseTagWrapStyle(tag string, scriptWrapStyle int) (wrapStyle int, updated bool) {
	cmds := parseTagCommands(tag)
	wrapStyle = scriptWrapStyle
	for _, cmd := range cmds {
		if cmd.name == "q" {
			updated = true
			if !cmd.hasVal || cmd.val < 0 || cmd.val > 3 {
				wrapStyle = scriptWrapStyle
			} else {
				wrapStyle = cmd.val
			}
		}
	}
	return wrapStyle, updated
}

// Dialogue represents a single dialogue or comment event in an ASS/SSA file
type Dialogue struct {
	Index          int           // 1-based index among translatable dialogue entries
	Type           string        // "Dialogue" or "Comment"
	Layer          string        // Layer (ASS) or Marked (SSA)
	Start          time.Duration // Start time
	End            time.Duration // End time
	Style          string        // Style name
	Name           string        // Actor or speaker name
	MarginL        string        // Left margin override
	MarginR        string        // Right margin override
	MarginV        string        // Vertical margin override
	Effect         string        // Special effect
	Text           string        // Full dialogue text
	CleanText      string        // Translatable text with tags replaced by placeholders
	Tags           []string      // Override tags extracted from text
	IsTranslatable bool          // True if the line contains translatable text
	Fields         []string      // Original raw fields to preserve custom format fields
}

// SetTranslatedText updates the dialogue's text with translated content, restoring tags
func (d *Dialogue) SetTranslatedText(translated string) {
	if !d.IsTranslatable {
		return
	}
	d.CleanText = translated
	// Escape only bare (unescaped) curly braces so they are rendered as literal text in ASS
	escaped := escapeBareBraces(translated)
	d.Text = restoreTags(escaped, d.Tags)
}

func escapeBareBraces(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '{' || c == '}' {
			if i == 0 || s[i-1] != '\\' {
				b.WriteByte('\\')
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}

// Event represents an entry in an [Events] section
type Event struct {
	IsDialogue bool
	Dialogue   *Dialogue
	RawLine    string
	Format     []string
}

// Section represents an arbitrary section in an ASS file
type Section struct {
	Header     string   // Section header e.g. "[Script Info]" or empty for preamble
	Lines      []string // Raw lines for non-Events sections
	IsEvents   bool     // True if this is an [Events] section
	FormatLine string   // The "Format: ..." line for this Events section
	Format     []string // Parsed field names for this Events section
	Events     []Event  // Events belonging strictly to this section
}

// File represents a parsed ASS/SSA subtitle document
type File struct {
	Sections []Section
}

type fieldIndices struct {
	layer   int
	start   int
	end     int
	style   int
	name    int
	marginL int
	marginR int
	marginV int
	effect  int
	text    int
}

// ParseASS parses an ASS/SSA subtitle from string content
func ParseASS(content string) (*File, error) {
	content = strings.TrimPrefix(content, "\ufeff")

	file := &File{
		Sections: []Section{},
	}

	reader := bufio.NewReader(strings.NewReader(content))
	var currentSection *Section
	var indices *fieldIndices
	dialogueIndex := 0

	for {
		line, errRead := reader.ReadString('\n')
		if errRead != nil && len(line) == 0 {
			if errRead != io.EOF {
				return nil, fmt.Errorf("failed to read ASS content: %w", errRead)
			}
			break
		}
		line = strings.TrimRight(line, "\r\n")
		trimmedLine := strings.TrimSpace(line)

		if strings.HasPrefix(trimmedLine, "[") && strings.HasSuffix(trimmedLine, "]") {
			if currentSection != nil {
				file.Sections = append(file.Sections, *currentSection)
			}
			isEvents := strings.EqualFold(trimmedLine, "[events]")
			currentSection = &Section{
				Header:   line,
				Lines:    []string{},
				IsEvents: isEvents,
				Events:   []Event{},
			}
			indices = nil
			continue
		}

		if currentSection == nil {
			currentSection = &Section{
				Header:   "",
				Lines:    []string{},
				IsEvents: false,
				Events:   []Event{},
			}
		}

		if !currentSection.IsEvents {
			currentSection.Lines = append(currentSection.Lines, line)
			continue
		}

		// Inside [Events] section
		if strings.HasPrefix(strings.ToLower(trimmedLine), "format:") {
			formatStr := strings.TrimSpace(line[strings.Index(line, ":")+1:])
			rawFields := strings.Split(formatStr, ",")
			currentSection.Format = make([]string, len(rawFields))
			for i, field := range rawFields {
				currentSection.Format[i] = strings.TrimSpace(field)
			}
			indices = resolveFieldIndices(currentSection.Format)
			currentSection.Events = append(currentSection.Events, Event{
				IsDialogue: false,
				RawLine:    line,
			})
			continue
		}

		// Check for Dialogue: or Comment:
		colonIdx := strings.Index(line, ":")
		if colonIdx != -1 {
			eventType := strings.TrimSpace(line[:colonIdx])
			lowerEventType := strings.ToLower(eventType)
			if lowerEventType == "dialogue" || lowerEventType == "comment" {
				if indices == nil {
					indices = defaultFieldIndices()
					currentSection.Format = defaultFormatFields()
					currentSection.FormatLine = "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text"
				}

				payload := strings.TrimLeft(line[colonIdx+1:], " \t")
				expectedFields := len(currentSection.Format)
				if expectedFields == 0 {
					expectedFields = 10
				}
				parts := strings.SplitN(payload, ",", expectedFields)
				if len(parts) >= expectedFields && indices.start >= 0 && indices.end >= 0 && indices.text >= 0 {
					startDuration, errStartDuration := ParseASSTimestamp(getField(parts, indices.start, "0:00:00.00"))
					if errStartDuration != nil {
						return nil, fmt.Errorf("failed to parse start timestamp %q in %s line: %w", getField(parts, indices.start, "0:00:00.00"), eventType, errStartDuration)
					}
					endDuration, errEndDuration := ParseASSTimestamp(getField(parts, indices.end, "0:00:00.00"))
					if errEndDuration != nil {
						return nil, fmt.Errorf("failed to parse end timestamp %q in %s line: %w", getField(parts, indices.end, "0:00:00.00"), eventType, errEndDuration)
					}

					textVal := ""
					if indices.text < len(parts) {
						textVal = parts[indices.text]
					}

					isDialogue := lowerEventType == "dialogue"
					if isDialogue {
						dialogueIndex++
					}

					protected, tags, isTranslatable := ExtractTags(textVal)

					dlg := &Dialogue{
						Index:          dialogueIndex,
						Type:           eventType,
						Layer:          getField(parts, indices.layer, "0"),
						Start:          startDuration,
						End:            endDuration,
						Style:          getField(parts, indices.style, "Default"),
						Name:           getField(parts, indices.name, ""),
						MarginL:        getField(parts, indices.marginL, "0"),
						MarginR:        getField(parts, indices.marginR, "0"),
						MarginV:        getField(parts, indices.marginV, "0"),
						Effect:         getField(parts, indices.effect, ""),
						Text:           textVal,
						CleanText:      protected,
						Tags:           tags,
						IsTranslatable: isTranslatable,
						Fields:         append([]string(nil), parts...),
					}

					currentSection.Events = append(currentSection.Events, Event{
						IsDialogue: isDialogue,
						Dialogue:   dlg,
						RawLine:    line,
						Format:     append([]string(nil), currentSection.Format...),
					})
					continue
				}
			}
		}

		// Non-dialogue line in [Events] section (comments, empty lines, etc.)
		currentSection.Events = append(currentSection.Events, Event{
			IsDialogue: false,
			RawLine:    line,
		})
	}

	if currentSection != nil {
		file.Sections = append(file.Sections, *currentSection)
	}

	return file, nil
}

func nextSpecialToken(s string, hasBrace bool) (idx int, tokenLen int, tokenType string) {
	n := len(s)
	for i := 0; i < n; i++ {
		c := s[i]
		if hasBrace && c == '{' {
			if i == 0 || s[i-1] != '\\' {
				return i, 1, "brace"
			}
		} else if c == '\\' {
			if i+1 < n {
				nxt := s[i+1]
				if nxt == 'N' || nxt == 'n' || nxt == 'h' {
					return i, 2, "layout"
				}
			}
		}
	}
	return -1, 0, ""
}

// ExtractTags parses text into protected placeholder format, isolating override tags and drawing commands
func ExtractTags(text string) (protected string, tags []string, isTranslatable bool) {
	text = strings.ReplaceAll(text, "[[ASSTAG", escapedTagPrefix)

	var result strings.Builder
	tagIdx := 0
	normalTextCount := 0
	noClosingBrace := false

	s := text
	for len(s) > 0 {
		openIdx, tokLen, tokType := nextSpecialToken(s, !noClosingBrace)
		if openIdx == -1 {
			result.WriteString(s)
			normalTextCount += len(strings.TrimSpace(s))
			break
		}

		if openIdx > 0 {
			chunk := s[:openIdx]
			result.WriteString(chunk)
			normalTextCount += len(strings.TrimSpace(chunk))
		}

		if tokType == "layout" {
			layoutTag := s[openIdx : openIdx+tokLen]
			placeholder := fmt.Sprintf("[[ASSTAG%d]]", tagIdx)
			result.WriteString(placeholder)
			tags = append(tags, layoutTag)
			tagIdx++
			s = s[openIdx+tokLen:]
			continue
		}

		// tokType == "brace"
		closeIdx := -1
		if !noClosingBrace {
			closeIdx = strings.Index(s[openIdx:], "}")
			if closeIdx == -1 {
				noClosingBrace = true
			}
		}

		if noClosingBrace {
			// Unclosed brace, treat '{' as normal literal character and continue scanning
			result.WriteString(s[openIdx : openIdx+1])
			normalTextCount += len(strings.TrimSpace(s[openIdx : openIdx+1]))
			s = s[openIdx+1:]
			continue
		}

		closeIdx += openIdx
		tag := s[openIdx : closeIdx+1]

		// Check if this tag activates drawing mode
		drawingOn, hasCmd := parseTagDrawingMode(tag)
		if hasCmd && drawingOn {
			// Atomic drawing unit: consume everything until drawing mode turns off (e.g. \p0) or EOF
			var drawingUnit strings.Builder
			drawingUnit.WriteString(tag)
			rem := s[closeIdx+1:]
			drawingOff := false

			for len(rem) > 0 {
				nextOpen := strings.Index(rem, "{")
				if nextOpen == -1 {
					drawingUnit.WriteString(rem)
					rem = ""
					break
				}

				if nextOpen > 0 {
					drawingUnit.WriteString(rem[:nextOpen])
					rem = rem[nextOpen:]
				}

				nextClose := strings.Index(rem, "}")
				if nextClose == -1 {
					drawingUnit.WriteString(rem)
					rem = ""
					break
				}

				nextTag := rem[:nextClose+1]
				drawingUnit.WriteString(nextTag)
				rem = rem[nextClose+1:]

				offDrawingOn, hasOffCmd := parseTagDrawingMode(nextTag)
				if hasOffCmd && !offDrawingOn {
					drawingOff = true
					break
				}
			}

			// If drawing mode was not explicitly closed, append {\p0} so text after placeholder stays in text mode
			if !drawingOff {
				drawingUnit.WriteString(`{\p0}`)
			}

			placeholder := fmt.Sprintf("[[ASSTAG%d]]", tagIdx)
			result.WriteString(placeholder)
			tags = append(tags, drawingUnit.String())
			tagIdx++

			s = rem
			continue
		}

		placeholder := fmt.Sprintf("[[ASSTAG%d]]", tagIdx)
		result.WriteString(placeholder)
		tags = append(tags, tag)
		tagIdx++

		s = s[closeIdx+1:]
	}

	isTranslatable = normalTextCount > 0
	return result.String(), tags, isTranslatable
}

// StripASSTagsToPlainText removes all ASS override tags and drawing data from text, returning clean plain text
func StripASSTagsToPlainText(rawText string, defaultWrapStyle ...int) (string, bool) {
	protected, tags, isTranslatable := ExtractTags(rawText)
	if !isTranslatable {
		return "", false
	}

	scriptWrapStyle := 0
	if len(defaultWrapStyle) > 0 {
		scriptWrapStyle = defaultWrapStyle[0]
	}
	activeWrapStyle := scriptWrapStyle

	clean := reASSTagPlaceholder.ReplaceAllStringFunc(protected, func(match string) string {
		sub := reASSTagPlaceholder.FindStringSubmatch(match)
		if len(sub) > 1 {
			idx, errAtoi := strconv.Atoi(sub[1])
			if errAtoi == nil && idx >= 0 && idx < len(tags) {
				tag := tags[idx]
				if tag == `\N` {
					return "\n"
				}
				if tag == `\n` {
					if activeWrapStyle == 2 {
						return "\n"
					}
					return " "
				}
				if tag == `\h` {
					return " "
				}
				if newWrap, updated := parseTagWrapStyle(tag, scriptWrapStyle); updated {
					activeWrapStyle = newWrap
				}
				return ""
			}
		}
		return ""
	})
	clean = strings.ReplaceAll(clean, `\{`, "{")
	clean = strings.ReplaceAll(clean, `\}`, "}")
	clean = strings.ReplaceAll(clean, escapedTagPrefix, "[[ASSTAG")
	clean = strings.TrimSpace(clean)
	if clean == "" {
		return "", false
	}
	return clean, true
}

func restoreTags(text string, tags []string) string {
	result := reASSTagPlaceholder.ReplaceAllStringFunc(text, func(match string) string {
		sub := reASSTagPlaceholder.FindStringSubmatch(match)
		if len(sub) > 1 {
			idx, errAtoi := strconv.Atoi(sub[1])
			if errAtoi == nil && idx >= 0 && idx < len(tags) {
				return tags[idx]
			}
		}
		return match
	})
	return strings.ReplaceAll(result, escapedTagPrefix, "[[ASSTAG")
}

// GetWrapStyle returns the script-level WrapStyle from the [Script Info] section (defaults to 0)
func (f *File) GetWrapStyle() int {
	for _, sec := range f.Sections {
		if strings.EqualFold(strings.TrimSpace(sec.Header), "[script info]") {
			for _, line := range sec.Lines {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, ";") {
					continue
				}
				colon := strings.Index(trimmed, ":")
				if colon != -1 {
					key := strings.TrimSpace(trimmed[:colon])
					if strings.EqualFold(key, "wrapstyle") {
						val, errAtoi := strconv.Atoi(strings.TrimSpace(trimmed[colon+1:]))
						if errAtoi == nil {
							return val
						}
					}
				}
			}
		}
	}
	return 0
}

// GetDialogues returns all translatable dialogue pointers in the file
func (f *File) GetDialogues() []*Dialogue {
	var dialogues []*Dialogue
	for sIdx := range f.Sections {
		if f.Sections[sIdx].IsEvents {
			for eIdx := range f.Sections[sIdx].Events {
				if f.Sections[sIdx].Events[eIdx].IsDialogue && f.Sections[sIdx].Events[eIdx].Dialogue != nil {
					dialogues = append(dialogues, f.Sections[sIdx].Events[eIdx].Dialogue)
				}
			}
		}
	}
	return dialogues
}

// ComposeASS converts an ASS File back into string content
func ComposeASS(file *File) string {
	var builder strings.Builder

	for _, section := range file.Sections {
		if section.Header != "" {
			builder.WriteString(section.Header)
			builder.WriteString("\n")
		}

		if !section.IsEvents {
			for _, line := range section.Lines {
				builder.WriteString(line)
				builder.WriteString("\n")
			}
			continue
		}

		// Events section
		hasFormatEvent := false
		for _, ev := range section.Events {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(ev.RawLine)), "format:") {
				hasFormatEvent = true
				break
			}
		}
		if !hasFormatEvent && section.FormatLine != "" {
			builder.WriteString(section.FormatLine)
			builder.WriteString("\n")
		}

		for _, event := range section.Events {
			if !event.IsDialogue || event.Dialogue == nil {
				builder.WriteString(event.RawLine)
				builder.WriteString("\n")
				continue
			}

			format := event.Format
			if len(format) == 0 {
				format = section.Format
			}
			indices := resolveFieldIndices(format)
			line := formatDialogueEvent(event.Dialogue, format, indices)
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}

	return builder.String()
}

func formatDialogueEvent(d *Dialogue, formatFields []string, indices *fieldIndices) string {
	if len(formatFields) == 0 {
		formatFields = defaultFormatFields()
		indices = defaultFieldIndices()
	}

	values := append([]string(nil), d.Fields...)
	if len(values) < len(formatFields) {
		extended := make([]string, len(formatFields))
		copy(extended, values)
		values = extended
	}

	setField := func(idx int, val string) {
		if idx >= 0 && idx < len(values) {
			values[idx] = val
		}
	}

	if indices.layer >= 0 {
		setField(indices.layer, d.Layer)
	}
	if indices.start >= 0 {
		setField(indices.start, FormatASSTimestamp(d.Start))
	}
	if indices.end >= 0 {
		setField(indices.end, FormatASSTimestamp(d.End))
	}
	if indices.style >= 0 {
		setField(indices.style, d.Style)
	}
	if indices.name >= 0 {
		setField(indices.name, d.Name)
	}
	if indices.marginL >= 0 {
		setField(indices.marginL, d.MarginL)
	}
	if indices.marginR >= 0 {
		setField(indices.marginR, d.MarginR)
	}
	if indices.marginV >= 0 {
		setField(indices.marginV, d.MarginV)
	}
	if indices.effect >= 0 {
		setField(indices.effect, d.Effect)
	}

	cleanText := strings.ReplaceAll(d.Text, "\r\n", `\N`)
	cleanText = strings.ReplaceAll(cleanText, "\n", `\N`)
	cleanText = strings.ReplaceAll(cleanText, "\r", `\N`)
	if indices.text >= 0 {
		setField(indices.text, cleanText)
	}

	return fmt.Sprintf("%s: %s", d.Type, strings.Join(values, ","))
}

func resolveFieldIndices(fields []string) *fieldIndices {
	if len(fields) == 0 {
		return defaultFieldIndices()
	}

	fi := &fieldIndices{
		layer:   -1,
		start:   -1,
		end:     -1,
		style:   -1,
		name:    -1,
		marginL: -1,
		marginR: -1,
		marginV: -1,
		effect:  -1,
		text:    -1,
	}

	for i, f := range fields {
		clean := strings.ToLower(strings.TrimSpace(f))
		switch clean {
		case "layer", "marked":
			fi.layer = i
		case "start":
			fi.start = i
		case "end":
			fi.end = i
		case "style":
			fi.style = i
		case "name":
			fi.name = i
		case "marginl":
			fi.marginL = i
		case "marginr":
			fi.marginR = i
		case "marginv":
			fi.marginV = i
		case "effect":
			fi.effect = i
		case "text":
			fi.text = i
		}
	}

	return fi
}

func defaultFieldIndices() *fieldIndices {
	return &fieldIndices{
		layer:   0,
		start:   1,
		end:     2,
		style:   3,
		name:    4,
		marginL: 5,
		marginR: 6,
		marginV: 7,
		effect:  8,
		text:    9,
	}
}

func defaultFormatFields() []string {
	return []string{"Layer", "Start", "End", "Style", "Name", "MarginL", "MarginR", "MarginV", "Effect", "Text"}
}

func getField(parts []string, idx int, defaultVal string) string {
	if idx >= 0 && idx < len(parts) {
		return strings.TrimSpace(parts[idx])
	}
	return defaultVal
}

// FormatASSTimestamp formats duration to ASS format "H:MM:SS.cc"
func FormatASSTimestamp(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	totalCentis := int64(d / (10 * time.Millisecond))
	hours := totalCentis / 360000
	minutes := (totalCentis % 360000) / 6000
	seconds := (totalCentis % 6000) / 100
	centis := totalCentis % 100

	return fmt.Sprintf("%d:%02d:%02d.%02d", hours, minutes, seconds, centis)
}

// ParseASSTimestamp parses ASS timestamp format "H:MM:SS.cc" with exact integer arithmetic
func ParseASSTimestamp(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	s = strings.Replace(s, ",", ".", 1)

	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid ASS timestamp: %s", s)
	}

	hours, errHours := strconv.Atoi(parts[0])
	if errHours != nil {
		return 0, fmt.Errorf("invalid hours in timestamp %s: %w", s, errHours)
	}
	if hours < 0 || hours > 10000 {
		return 0, fmt.Errorf("hours out of range in timestamp %s: %d", s, hours)
	}

	minutes, errMinutes := strconv.Atoi(parts[1])
	if errMinutes != nil {
		return 0, fmt.Errorf("invalid minutes in timestamp %s: %w", s, errMinutes)
	}
	if minutes < 0 || minutes >= 60 {
		return 0, fmt.Errorf("minutes out of range in timestamp %s: %d", s, minutes)
	}

	secParts := strings.Split(parts[2], ".")
	if len(secParts) > 2 {
		return 0, fmt.Errorf("invalid seconds format in timestamp %s", s)
	}
	seconds, errSeconds := strconv.Atoi(secParts[0])
	if errSeconds != nil {
		return 0, fmt.Errorf("invalid seconds in timestamp %s: %w", s, errSeconds)
	}
	if seconds < 0 || seconds >= 60 {
		return 0, fmt.Errorf("seconds out of range in timestamp %s: %d", s, seconds)
	}

	var fracNanos int64
	if len(secParts) == 2 {
		fracStr := secParts[1]
		if len(fracStr) == 0 {
			return 0, fmt.Errorf("empty fraction in timestamp %s", s)
		}
		for _, r := range fracStr {
			if r < '0' || r > '9' {
				return 0, fmt.Errorf("invalid characters in fraction of timestamp %s", s)
			}
		}
		if len(fracStr) == 1 {
			val, _ := strconv.Atoi(fracStr)
			fracNanos = int64(val) * 100000000
		} else if len(fracStr) == 2 {
			val, _ := strconv.Atoi(fracStr)
			fracNanos = int64(val) * 10000000
		} else {
			val, _ := strconv.Atoi(fracStr[:3])
			fracNanos = int64(val) * 1000000
		}
	}

	totalDuration := time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second +
		time.Duration(fracNanos)

	if totalDuration < 0 {
		return 0, fmt.Errorf("timestamp duration overflow: %s", s)
	}

	return totalDuration, nil
}

// SplitASSTags splits text into leading override tags, translatable text, and trailing tags (legacy helper)
func SplitASSTags(text string) (prefix string, clean string, suffix string) {
	s := text

	for strings.HasPrefix(s, "{") {
		closeIdx := strings.Index(s, "}")
		if closeIdx == -1 {
			break
		}
		prefix += s[:closeIdx+1]
		s = s[closeIdx+1:]
	}

	for strings.HasSuffix(s, "}") {
		lastOpenIdx := strings.LastIndex(s, "{")
		if lastOpenIdx == -1 {
			break
		}
		suffix = s[lastOpenIdx:] + suffix
		s = s[:lastOpenIdx]
	}

	clean = s
	return prefix, clean, suffix
}
