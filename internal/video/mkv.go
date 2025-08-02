package video

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/luispater/gemini-srt-translator-go/pkg/matroska"

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

	// Create demuxer
	demuxer, err := matroska.NewDemuxer(file)
	if err != nil {
		return errors.NewFileError("failed to create Matroska demuxer", err)
	}
	defer demuxer.Close()

	// Get file info for timecode scale
	fileInfo, err := demuxer.GetFileInfo()
	if err != nil {
		return errors.NewFileError("failed to get file info", err)
	}
	p.timecodescale = fileInfo.TimecodeScale

	// Get number of tracks
	numTracks, err := demuxer.GetNumTracks()
	if err != nil {
		return errors.NewFileError("failed to get number of tracks", err)
	}

	// Extract subtitle tracks (with deduplication)
	seenTracks := make(map[uint8]bool)
	for i := uint(0); i < numTracks; i++ {
		trackInfo, errGetTrackInfo := demuxer.GetTrackInfo(i)
		if errGetTrackInfo != nil {
			continue // Skip tracks we can't read
		}

		// Only process subtitle tracks
		if trackInfo.Type == matroska.TypeSubtitle && strings.HasPrefix(trackInfo.CodecID, "S_TEXT") {
			// Skip duplicate track numbers
			if seenTracks[trackInfo.Number] {
				continue
			}
			seenTracks[trackInfo.Number] = true

			track := SubtitleTrack{
				Number:   int(trackInfo.Number),
				Language: trackInfo.Language,
				Name:     trackInfo.Name,
				Codec:    trackInfo.CodecID,
				Entries:  []SubtitleEntry{},
			}
			p.tracks = append(p.tracks, track)
		}
	}

	// Extract subtitle packets
	err = p.extractSubtitlePackets(demuxer)
	if err != nil {
		return errors.NewFileError("failed to extract subtitle packets", err)
	}

	return nil
}

// extractSubtitlePackets extracts subtitle packets from the demuxer
func (p *MKVParser) extractSubtitlePackets(demuxer *matroska.Demuxer) error {
	// Create a map for quick track lookup
	trackMap := make(map[uint8]*SubtitleTrack)
	for i := range p.tracks {
		trackMap[uint8(p.tracks[i].Number)] = &p.tracks[i]
	}

	// Read all packets
	for {
		packet, err := demuxer.ReadPacket()
		if err != nil {
			if err == io.EOF {
				break // End of file reached
			}
			return fmt.Errorf("failed to read packet: %w", err)
		}

		// Find the corresponding subtitle track
		track, exists := trackMap[packet.Track]
		if !exists {
			continue // Not a subtitle track we're interested in
		}

		// Convert packet data to text (assuming UTF-8)
		text := strings.TrimSpace(string(packet.Data))
		if text == "" {
			continue // Skip empty packets
		}

		// Calculate timing using the file's timecode scale
		startTime := time.Duration(packet.StartTime) * time.Duration(p.timecodescale)
		endTime := time.Duration(packet.EndTime) * time.Duration(p.timecodescale)

		// Create subtitle entry
		entry := SubtitleEntry{
			Start:    startTime,
			End:      endTime,
			Text:     text,
			Duration: endTime - startTime,
		}

		track.Entries = append(track.Entries, entry)
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
