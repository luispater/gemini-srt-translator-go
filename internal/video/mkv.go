package video

import (
	"bufio"
	stdErrors "errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/luispater/matroska-go"

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

// Parse parses the MKV file and extracts subtitle tracks and packets.
func (p *MKVParser) Parse() error {
	return p.parse(true)
}

// ParseTracks parses only MKV track metadata without reading subtitle packets.
func (p *MKVParser) ParseTracks() error {
	return p.parse(false)
}

func (p *MKVParser) parse(includePackets bool) (returnErr error) {
	file, errOpen := os.Open(p.filename)
	if errOpen != nil {
		return errors.NewFileError(fmt.Sprintf("failed to open MKV file: %s", p.filename), errOpen)
	}
	defer func() {
		if errClose := file.Close(); errClose != nil {
			returnErr = stdErrors.Join(returnErr, errors.NewFileError("failed to close MKV file", errClose))
		}
	}()

	demuxer, errDemuxer := matroska.NewDemuxer(file)
	if errDemuxer != nil {
		return errors.NewFileError("failed to create Matroska demuxer", errDemuxer)
	}
	defer demuxer.Close()

	fileInfo, errFileInfo := demuxer.GetFileInfo()
	if errFileInfo != nil {
		return errors.NewFileError("failed to get file info", errFileInfo)
	}
	p.timecodescale = fileInfo.TimecodeScale

	numTracks, errNumTracks := demuxer.GetNumTracks()
	if errNumTracks != nil {
		return errors.NewFileError("failed to get number of tracks", errNumTracks)
	}

	p.tracks = nil
	seenTracks := make(map[uint8]bool)
	for i := uint(0); i < numTracks; i++ {
		trackInfo, errGetTrackInfo := demuxer.GetTrackInfo(i)
		if errGetTrackInfo != nil {
			continue
		}
		if trackInfo.Type != matroska.TypeSubtitle || !strings.HasPrefix(trackInfo.CodecID, "S_TEXT") {
			continue
		}
		if seenTracks[trackInfo.Number] {
			continue
		}
		seenTracks[trackInfo.Number] = true
		p.tracks = append(p.tracks, SubtitleTrack{
			Number:   int(trackInfo.Number),
			Language: trackInfo.Language,
			Name:     trackInfo.Name,
			Codec:    trackInfo.CodecID,
			Entries:  []SubtitleEntry{},
		})
	}

	if !includePackets {
		return nil
	}
	if errExtract := p.extractSubtitlePackets(demuxer); errExtract != nil {
		return errors.NewFileError("failed to extract subtitle packets", errExtract)
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
			if err == io.EOF || strings.Contains(err.Error(), "EOF") {
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
		// startTime := time.Duration(packet.StartTime) * time.Duration(p.timecodescale)
		// endTime := time.Duration(packet.EndTime) * time.Duration(p.timecodescale)
		startTime := time.Duration(packet.StartTime)
		endTime := time.Duration(packet.EndTime)

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

// SelectBestSubtitleTrack selects an English non-SDH track when available.
func SelectBestSubtitleTrack(tracks []SubtitleTrack) *SubtitleTrack {
	var englishTracks []SubtitleTrack
	for _, track := range tracks {
		if isEnglishTrack(track.Language) {
			englishTracks = append(englishTracks, track)
		}
	}
	if len(englishTracks) == 0 {
		if len(tracks) == 0 {
			return nil
		}
		selected := tracks[0]
		return &selected
	}
	for _, track := range englishTracks {
		if !isSDHTrack(track.Name) {
			selected := track
			return &selected
		}
	}
	selected := englishTracks[0]
	return &selected
}

// SelectBestEnglishTrack selects the best English subtitle track (non-SDH preferred).
func (p *MKVParser) SelectBestEnglishTrack() (*SubtitleTrack, error) {
	selected := SelectBestSubtitleTrack(p.tracks)
	if selected == nil {
		return nil, errors.NewValidationError("no subtitle tracks found in MKV", nil)
	}
	return selected, nil
}

// ExtractToSRT extracts a subtitle track to SRT format
func (p *MKVParser) ExtractToSRT(track *SubtitleTrack, outputPath string) (returnErr error) {
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
			returnErr = stdErrors.Join(returnErr, errors.NewFileError("failed to close SRT output file", errClose))
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

// InspectSubtitleTracks returns MKV subtitle track metadata for interactive selection.
func InspectSubtitleTracks(mkvPath string) ([]SubtitleTrack, error) {
	if errValidate := validateMKVPath(mkvPath); errValidate != nil {
		return nil, errValidate
	}

	parser := NewMKVParser(mkvPath)
	if errParse := parser.ParseTracks(); errParse != nil {
		return nil, errParse
	}
	tracks := parser.GetSubtitleTracks()
	if len(tracks) == 0 {
		return nil, errors.NewValidationError("no subtitle tracks found in MKV", nil)
	}
	return tracks, nil
}

// ExtractSubtitlesFromMKV extracts the selected subtitle track and returns its SRT path.
func ExtractSubtitlesFromMKV(mkvPath string, trackNumber int) (string, error) {
	if errValidate := validateMKVPath(mkvPath); errValidate != nil {
		return "", errValidate
	}

	parser := NewMKVParser(mkvPath)
	if errParse := parser.Parse(); errParse != nil {
		return "", errParse
	}

	tracks := parser.GetSubtitleTracks()
	if len(tracks) == 0 {
		return "", errors.NewValidationError("no subtitle tracks found in MKV", nil)
	}

	var selected *SubtitleTrack
	for i := range tracks {
		if tracks[i].Number == trackNumber {
			selected = &tracks[i]
			break
		}
	}
	if selected == nil && trackNumber == 0 {
		bestTrack, errBestTrack := parser.SelectBestEnglishTrack()
		if errBestTrack != nil {
			return "", errBestTrack
		}
		selected = bestTrack
	}
	if selected == nil {
		return "", errors.NewValidationError("selected subtitle track was not found", nil).WithContext("track_number", trackNumber)
	}

	baseName := strings.TrimSuffix(filepath.Base(mkvPath), filepath.Ext(mkvPath))
	tempFile, errCreateTemp := os.CreateTemp("", "gst-"+baseName+"-*.srt")
	if errCreateTemp != nil {
		return "", errors.NewFileError("failed to create temporary subtitle file", errCreateTemp)
	}
	tempPath := tempFile.Name()
	if errCloseTemp := tempFile.Close(); errCloseTemp != nil {
		_ = os.Remove(tempPath)
		return "", errors.NewFileError("failed to close temporary subtitle file", errCloseTemp)
	}
	if errExtract := parser.ExtractToSRT(selected, tempPath); errExtract != nil {
		_ = os.Remove(tempPath)
		return "", errExtract
	}
	return tempPath, nil
}

func validateMKVPath(mkvPath string) error {
	if !strings.HasSuffix(strings.ToLower(mkvPath), ".mkv") {
		return errors.NewValidationError("file is not an MKV file", nil).WithContext("file_path", mkvPath)
	}
	if _, errStat := os.Stat(mkvPath); errStat != nil {
		return errors.NewFileError(fmt.Sprintf("MKV file does not exist: %s", mkvPath), errStat)
	}
	return nil
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
