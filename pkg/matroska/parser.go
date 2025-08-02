package matroska

import (
	"errors"
	"fmt"
	"io"
)

type Parser struct {
	reader      *EBMLReader
	segmentPos  uint64
	segmentSize uint64
	segmentInfo *SegmentInfo
	tracks      []*TrackInfo
	attachments []*Attachment
	chapters    []*Chapter
	tags        []*Tag
	cues        []*Cue
	clusters    []*ClusterInfo

	avoidSeeks bool
}

type ClusterInfo struct {
	Position uint64
	Timecode uint64
	PrevSize uint64
	Size     uint64
}

func NewParser(r io.ReadSeeker) (*Parser, error) {
	reader := NewEBMLReader(r)

	ebmlHeader, err := reader.ReadElement()
	if err != nil {
		return nil, fmt.Errorf("failed to read EBML header: %w", err)
	}

	if ebmlHeader.ID != EBMLHeaderID {
		return nil, errors.New("not a valid EBML file")
	}

	if err = validateEBMLHeader(reader, ebmlHeader); err != nil {
		return nil, fmt.Errorf("invalid EBML header: %w", err)
	}

	segmentElement, err := reader.ReadElement()
	if err != nil {
		return nil, fmt.Errorf("failed to read segment: %w", err)
	}

	if segmentElement.ID != SegmentID {
		return nil, errors.New("expected Segment element")
	}

	parser := &Parser{
		reader:      reader,
		segmentPos:  segmentElement.Offset + getElementHeaderSize(segmentElement),
		segmentSize: segmentElement.Size,
	}

	if err = parser.parseSegment(); err != nil {
		return nil, fmt.Errorf("failed to parse segment: %w", err)
	}

	return parser, nil
}

func NewStreamingParser(r io.Reader) (*Parser, error) {
	newFakeSeeker := &fakeSeeker{r: r}
	parser, err := NewParser(newFakeSeeker)
	if err != nil {
		return nil, err
	}
	parser.avoidSeeks = true
	return parser, nil
}

func validateEBMLHeader(reader *EBMLReader, header *EBMLElement) error {
	// Create a new reader for the EBML header data
	headerReader := NewEBMLReader(&bytesReader{data: header.Data})
	children, err := ParseEBMLChildren(headerReader, uint64(len(header.Data)))
	if err != nil {
		return err
	}

	var docType string
	var docTypeVersion uint64 = 1

	for _, child := range children {
		switch child.ID {
		case DocTypeID:
			docType = child.ReadString()
		case DocTypeVersionID:
			docTypeVersion, _ = child.ReadUint()
		}
	}

	if docType != "matroska" && docType != "webm" {
		return fmt.Errorf("unsupported document type: %s", docType)
	}

	if docTypeVersion < 1 {
		return fmt.Errorf("unsupported document version: %d", docTypeVersion)
	}

	return nil
}

func getElementHeaderSize(element *EBMLElement) uint64 {
	return element.HeaderSize
}

func (p *Parser) parseSegment() error {
	originalPos := p.reader.Position()
	defer func() {
		if p.avoidSeeks {
			return
		}
		_ = p.reader.Seek(originalPos)
	}()

	if err := p.reader.Seek(p.segmentPos); err != nil {
		return err
	}

	endPos := p.segmentPos + p.segmentSize

	for p.reader.Position() < endPos {
		element, err := p.reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		switch element.ID {
		case SeekHeadID:
			if err = p.parseSeekHead(element); err != nil {
				return fmt.Errorf("failed to parse SeekHead: %w", err)
			}
		case SegmentInfoID:
			if err = p.parseSegmentInfo(element); err != nil {
				return fmt.Errorf("failed to parse SegmentInfo: %w", err)
			}
		case TracksID:
			if err = p.parseTracks(element); err != nil {
				return fmt.Errorf("failed to parse Tracks: %w", err)
			}
		case CuesID:
			if err = p.parseCues(element); err != nil {
				return fmt.Errorf("failed to parse Cues: %w", err)
			}
		case AttachmentsID:
			if err = p.parseAttachments(element); err != nil {
				return fmt.Errorf("failed to parse Attachments: %w", err)
			}
		case ChaptersID:
			if err = p.parseChapters(element); err != nil {
				return fmt.Errorf("failed to parse Chapters: %w", err)
			}
		case TagsID:
			if err = p.parseTags(element); err != nil {
				return fmt.Errorf("failed to parse Tags: %w", err)
			}
		case ClusterID:
			clusterInfo, errParseClusterInfo := p.parseClusterInfo(element)
			if errParseClusterInfo != nil {
				return fmt.Errorf("failed to parse Cluster: %w", errParseClusterInfo)
			}
			p.clusters = append(p.clusters, clusterInfo)

			if p.reader.Position() < endPos {
				_ = p.reader.Seek(element.Offset + getElementHeaderSize(element) + element.Size)
			}
		default:
			if p.reader.Position() < endPos {
				_ = p.reader.Seek(element.Offset + getElementHeaderSize(element) + element.Size)
			}
		}
	}

	return nil
}

func (p *Parser) parseSeekHead(element *EBMLElement) error {
	reader := NewEBMLReader(&bytesReader{data: element.Data})

	for reader.Position() < uint64(len(element.Data)) {
		seekElement, err := reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if seekElement.ID == SeekID {
			if err = p.parseSeek(seekElement); err != nil {
				return err
			}
		}
	}

	return nil
}

func (p *Parser) parseSeek(element *EBMLElement) error {
	reader := NewEBMLReader(&bytesReader{data: element.Data})

	var seekID uint32
	var seekPosition uint64

	for reader.Position() < uint64(len(element.Data)) {
		child, err := reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		switch child.ID {
		case SeekIDElementID:
			id, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			seekID = uint32(id)
		case SeekPositionID:
			pos, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			seekPosition = pos
		}
	}

	if !p.avoidSeeks && seekID != 0 && seekPosition != 0 {
		absolutePos := p.segmentPos + seekPosition
		if err := p.parseElementAtPosition(seekID, absolutePos); err != nil {
			return err
		}
	}

	return nil
}

func (p *Parser) parseElementAtPosition(elementID uint32, position uint64) error {
	originalPos := p.reader.Position()
	defer func() {
		_ = p.reader.Seek(originalPos)
	}()

	if err := p.reader.Seek(position); err != nil {
		return err
	}

	element, err := p.reader.ReadElement()
	if err != nil {
		return err
	}

	if element.ID != elementID {
		return fmt.Errorf("expected element ID %x, got %x", elementID, element.ID)
	}

	switch elementID {
	case SegmentInfoID:
		return p.parseSegmentInfo(element)
	case TracksID:
		return p.parseTracks(element)
	case CuesID:
		return p.parseCues(element)
	case AttachmentsID:
		return p.parseAttachments(element)
	case ChaptersID:
		return p.parseChapters(element)
	case TagsID:
		return p.parseTags(element)
	}

	return nil
}

type bytesReader struct {
	data []byte
	pos  int64
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.pos >= int64(len(r.data)) {
		return 0, io.EOF
	}

	n := copy(p, r.data[r.pos:])
	r.pos += int64(n)
	return n, nil
}

func (r *bytesReader) Seek(offset int64, whence int) (int64, error) {
	var newPos int64

	switch whence {
	case io.SeekStart:
		newPos = offset
	case io.SeekCurrent:
		newPos = r.pos + offset
	case io.SeekEnd:
		newPos = int64(len(r.data)) + offset
	default:
		return 0, errors.New("invalid whence")
	}

	if newPos < 0 {
		return 0, errors.New("negative position")
	}

	r.pos = newPos
	return newPos, nil
}
