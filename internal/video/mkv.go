package video

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/remko/go-mkvparse"

	"github.com/luispater/gemini-srt-translator-go/pkg/errors"
	"github.com/luispater/gemini-srt-translator-go/pkg/srt"
)

// SubtitleTrack represents a subtitle track in an MKV file
type SubtitleTrack struct {
	Number   int
	Language string
	Name     string
	Codec    string
	Entries  []SubtitleEntry
}

// SubtitleEntry represents a single subtitle entry
type SubtitleEntry struct {
	Start    time.Duration
	End      time.Duration
	Text     string
	Duration time.Duration
}

// MKVParser handles parsing MKV files for subtitle extraction
type MKVParser struct {
	filename      string
	tracks        []SubtitleTrack
	currentTrack  *SubtitleTrack
	trackNumber   int
	timecodescale uint64
}

// NewMKVParser creates a new MKV parser
func NewMKVParser(filename string) *MKVParser {
	return &MKVParser{
		filename:      filename,
		tracks:        []SubtitleTrack{},
		timecodescale: 1000000, // Default timecode scale (nanoseconds)
	}
}

// Parse parses the MKV file and extracts subtitle tracks
func (p *MKVParser) Parse() error {
	file, err := os.Open(p.filename)
	if err != nil {
		return errors.NewFileError(fmt.Sprintf("failed to open MKV file: %s", p.filename), err)
	}
	defer func() {
		if errClose := file.Close(); errClose != nil {
			// Log error but don't fail the operation
		}
	}()

	handler := &mkvHandler{parser: p}

	if errParse := mkvparse.Parse(file, handler); errParse != nil {
		return errors.NewFileError("failed to parse MKV file", errParse)
	}

	return nil
}

// GetSubtitleTracks returns all available subtitle tracks
func (p *MKVParser) GetSubtitleTracks() []SubtitleTrack {
	return p.tracks
}

// SelectBestEnglishTrack selects the best English subtitle track (non-SDH preferred)
func (p *MKVParser) SelectBestEnglishTrack() (*SubtitleTrack, error) {
	var englishTracks []SubtitleTrack

	// Filter for English tracks
	for _, track := range p.tracks {
		if isEnglishTrack(track.Language) {
			englishTracks = append(englishTracks, track)
		}
	}

	if len(englishTracks) == 0 {
		if len(p.tracks) > 0 {
			return &p.tracks[0], nil
		}
	}

	// If only one track, return it
	if len(englishTracks) == 1 {
		return &englishTracks[0], nil
	}

	// Prefer non-SDH tracks
	for _, track := range englishTracks {
		if !isSDHTrack(track.Name) {
			return &track, nil
		}
	}

	// If all are SDH or no preference, return the first one
	return &englishTracks[0], nil
}

// ExtractToSRT extracts a subtitle track to SRT format
func (p *MKVParser) ExtractToSRT(track *SubtitleTrack, outputPath string) error {
	if len(track.Entries) == 0 {
		return errors.NewValidationError("subtitle track is empty", nil)
	}

	// Convert subtitle entries to SRT format
	var subtitles []srt.Subtitle
	for i, entry := range track.Entries {
		subtitle := srt.Subtitle{
			Index:   i + 1,
			Start:   entry.Start,
			End:     entry.End,
			Content: entry.Text,
		}
		subtitles = append(subtitles, subtitle)
	}

	// Generate SRT content
	srtContent := srt.ComposeSRT(subtitles)

	// Write to file
	file, err := os.Create(outputPath)
	if err != nil {
		return errors.NewFileError(fmt.Sprintf("failed to create output file: %s", outputPath), err)
	}
	defer func() {
		if errClose := file.Close(); errClose != nil {
			// Log error but don't fail the operation
		}
	}()

	writer := bufio.NewWriter(file)
	if _, errWrite := writer.WriteString(srtContent); errWrite != nil {
		return errors.NewFileError("failed to write SRT content", errWrite)
	}

	if errFlush := writer.Flush(); errFlush != nil {
		return errors.NewFileError("failed to flush SRT content", errFlush)
	}

	return nil
}

// ExtractSubtitlesFromMKV extracts subtitles from MKV file and returns the path to extracted SRT
func ExtractSubtitlesFromMKV(mkvPath string) (string, error) {
	// Validate input file
	if !strings.HasSuffix(strings.ToLower(mkvPath), ".mkv") {
		return "", errors.NewValidationError("file is not an MKV file", nil).WithContext("file_path", mkvPath)
	}

	if _, err := os.Stat(mkvPath); os.IsNotExist(err) {
		return "", errors.NewFileError(fmt.Sprintf("MKV file does not exist: %s", mkvPath), err)
	}

	// Create parser and parse the file
	parser := NewMKVParser(mkvPath)
	if err := parser.Parse(); err != nil {
		return "", err
	}

	// Select the best English subtitle track
	track, err := parser.SelectBestEnglishTrack()
	if err != nil {
		return "", err
	}

	// Generate output path
	baseName := strings.TrimSuffix(filepath.Base(mkvPath), filepath.Ext(mkvPath))
	outputPath := filepath.Join(filepath.Dir(mkvPath), baseName+"_extracted.srt")

	// Extract to SRT
	if err = parser.ExtractToSRT(track, outputPath); err != nil {
		return "", err
	}

	return outputPath, nil
}

// isEnglishTrack checks if a track is in English
func isEnglishTrack(language string) bool {
	language = strings.ToLower(language)
	return language == "en" || language == "eng" || language == "english"
}

// isSDHTrack checks if a track is marked as SDH (Subtitles for the Deaf and Hard of hearing)
func isSDHTrack(name string) bool {
	name = strings.ToLower(name)
	return strings.Contains(name, "sdh") ||
		strings.Contains(name, "deaf") ||
		strings.Contains(name, "hard of hearing") ||
		strings.Contains(name, "cc") ||
		strings.Contains(name, "closed caption")
}

// mkvHandler implements mkvparse.Handler for parsing MKV files
type mkvHandler struct {
	parser       *MKVParser
	inTracks     bool
	inTrackEntry bool
	inCluster    bool
	inBlock      bool
	currentBlock *blockInfo
	clusterTime  uint64
}

type blockInfo struct {
	trackNumber int
	timecode    int16
	data        []byte
	duration    uint64
}

// HandleMasterBegin handles the beginning of master elements
func (h *mkvHandler) HandleMasterBegin(id mkvparse.ElementID, _ mkvparse.ElementInfo) (bool, error) {
	switch id {
	case mkvparse.TracksElement:
		h.inTracks = true
	case mkvparse.TrackEntryElement:
		h.inTrackEntry = true
		h.parser.currentTrack = &SubtitleTrack{}
	case mkvparse.ClusterElement:
		h.inCluster = true
	case mkvparse.BlockGroupElement, mkvparse.SimpleBlockElement:
		h.inBlock = true
		h.currentBlock = &blockInfo{}
	}
	return true, nil
}

// HandleMasterEnd handles the end of master elements
func (h *mkvHandler) HandleMasterEnd(id mkvparse.ElementID, _ mkvparse.ElementInfo) error {
	switch id {
	case mkvparse.TracksElement:
		h.inTracks = false
	case mkvparse.TrackEntryElement:
		h.inTrackEntry = false
		// Add completed track if it's a subtitle track
		if h.parser.currentTrack != nil && h.parser.currentTrack.Codec != "" {
			h.parser.tracks = append(h.parser.tracks, *h.parser.currentTrack)
		}
		h.parser.currentTrack = nil
	case mkvparse.ClusterElement:
		h.inCluster = false
	case mkvparse.BlockGroupElement, mkvparse.SimpleBlockElement:
		h.inBlock = false
		h.processBlock()
		h.currentBlock = nil
	}
	return nil
}

// HandleString handles string elements
func (h *mkvHandler) HandleString(id mkvparse.ElementID, value string, _ mkvparse.ElementInfo) error {
	if h.inTrackEntry && h.parser.currentTrack != nil {
		switch id {
		case mkvparse.CodecIDElement:
			// Only process subtitle codecs
			if strings.HasPrefix(value, "S_TEXT") {
				h.parser.currentTrack.Codec = value
			}
		case mkvparse.LanguageElement:
			h.parser.currentTrack.Language = value
		case mkvparse.NameElement:
			h.parser.currentTrack.Name = value
		}
	}
	return nil
}

// HandleInteger handles integer elements
func (h *mkvHandler) HandleInteger(id mkvparse.ElementID, value int64, _ mkvparse.ElementInfo) error {
	switch id {
	case mkvparse.TimecodeScaleElement:
		h.parser.timecodescale = uint64(value)
	case mkvparse.TimecodeElement:
		if h.inCluster {
			h.clusterTime = uint64(value)
		}
	case mkvparse.TrackNumberElement:
		if h.inTrackEntry && h.parser.currentTrack != nil {
			h.parser.currentTrack.Number = int(value)
		}
	case mkvparse.TrackTypeElement:
		// Skip non-subtitle tracks
		if h.inTrackEntry && h.parser.currentTrack != nil && value != 17 { // 17 is subtitle track type
			h.parser.currentTrack = nil
		}
	case mkvparse.BlockDurationElement:
		if h.currentBlock != nil {
			h.currentBlock.duration = uint64(value)
		}
	}
	return nil
}

// HandleBinary handles binary elements
func (h *mkvHandler) HandleBinary(id mkvparse.ElementID, value []byte, _ mkvparse.ElementInfo) error {
	if id == mkvparse.BlockElement || id == mkvparse.SimpleBlockElement {
		if h.currentBlock != nil {
			h.parseBlockData(value)
		}
	}
	return nil
}

// parseBlockData parses block data to extract subtitle information
func (h *mkvHandler) parseBlockData(data []byte) {
	if len(data) < 4 {
		return
	}

	// Parse track number (variable length)
	trackNum, offset := parseVariableInt(data)
	if offset >= len(data) {
		return
	}

	// Parse timecode (2 bytes)
	if offset+2 >= len(data) {
		return
	}
	timecode := int16(data[offset])<<8 | int16(data[offset+1])
	offset += 2

	// Skip flags (1 byte)
	if offset+1 >= len(data) {
		return
	}
	offset++

	// Extract payload
	payload := data[offset:]

	h.currentBlock.trackNumber = int(trackNum)
	h.currentBlock.timecode = timecode
	h.currentBlock.data = payload
}

// processBlock processes a completed block
func (h *mkvHandler) processBlock() {
	if h.currentBlock == nil {
		return
	}

	// Find the corresponding track
	var track *SubtitleTrack
	for i := range h.parser.tracks {
		if h.parser.tracks[i].Number == h.currentBlock.trackNumber {
			track = &h.parser.tracks[i]
			break
		}
	}

	if track == nil || track.Codec == "" {
		return
	}

	// Calculate timing
	blockTime := time.Duration(h.clusterTime+uint64(h.currentBlock.timecode)) * time.Duration(h.parser.timecodescale)
	duration := time.Duration(h.currentBlock.duration) * time.Duration(h.parser.timecodescale)

	// Convert payload to text (assuming UTF-8)
	text := strings.TrimSpace(string(h.currentBlock.data))
	if text == "" {
		return
	}

	// Create subtitle entry
	entry := SubtitleEntry{
		Start:    blockTime,
		End:      blockTime + duration,
		Text:     text,
		Duration: duration,
	}

	track.Entries = append(track.Entries, entry)
}

// parseVariableInt parses a variable-length integer as used in EBML
func parseVariableInt(data []byte) (uint64, int) {
	if len(data) == 0 {
		return 0, 0
	}

	// Find the length by looking at the first bit pattern
	firstByte := data[0]
	length := 1
	mask := uint8(0x80)

	for length <= 8 && (firstByte&mask) == 0 {
		length++
		mask >>= 1
	}

	if length > len(data) || length > 8 {
		return 0, 0
	}

	// Extract the value
	result := uint64(firstByte & (mask - 1))
	for i := 1; i < length; i++ {
		result = (result << 8) | uint64(data[i])
	}

	return result, length
}

// HandleFloat handles float elements
func (h *mkvHandler) HandleFloat(_ mkvparse.ElementID, _ float64, _ mkvparse.ElementInfo) error {
	// Not needed for subtitle extraction
	return nil
}

// HandleDate handles date elements
func (h *mkvHandler) HandleDate(_ mkvparse.ElementID, _ time.Time, _ mkvparse.ElementInfo) error {
	// Not needed for subtitle extraction
	return nil
}
