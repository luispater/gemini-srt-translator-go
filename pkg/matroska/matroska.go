package matroska

import (
	"fmt"
	"io"

	"github.com/pborman/uuid"
)

type Demuxer struct {
	parser       *Parser
	packetReader *PacketReader
	key          string
	closed       bool
}

func NewDemuxer(r io.ReadSeeker) (*Demuxer, error) {
	return newDemuxerWithFlag(r, false)
}

func NewStreamingDemuxer(r io.Reader) (*Demuxer, error) {
	return newDemuxerWithFlag(r, true)
}

func newDemuxerWithFlag(r io.Reader, streaming bool) (*Demuxer, error) {
	var parser *Parser
	var err error

	if streaming {
		readSeeker, ok := r.(io.ReadSeeker)
		if !ok {
			readSeeker = &fakeSeeker{r: r}
		}
		parser, err = NewStreamingParser(readSeeker)
	} else {
		readSeeker, ok := r.(io.ReadSeeker)
		if !ok {
			return nil, fmt.Errorf("non-seeking reader not supported for regular demuxer")
		}
		parser, err = NewParser(readSeeker)
	}

	if err != nil {
		return nil, err
	}

	demuxer := &Demuxer{
		parser:       parser,
		packetReader: NewPacketReader(parser),
		key:          uuid.New(),
	}

	return demuxer, nil
}

func (d *Demuxer) Close() {
	if d.closed {
		return
	}
	d.closed = true
	d.parser = nil
	d.packetReader = nil
}

func (d *Demuxer) GetNumTracks() (uint, error) {
	if d.closed {
		return 0, fmt.Errorf("demuxer is closed")
	}

	if len(d.parser.tracks) == 0 {
		return 0, fmt.Errorf("no tracks found")
	}

	return uint(len(d.parser.tracks)), nil
}

func (d *Demuxer) GetTrackInfo(track uint) (*TrackInfo, error) {
	if d.closed {
		return nil, fmt.Errorf("demuxer is closed")
	}

	if int(track) >= len(d.parser.tracks) {
		return nil, fmt.Errorf("track index out of range")
	}

	trackInfo := d.parser.tracks[track]
	result := &TrackInfo{}
	*result = *trackInfo

	if len(trackInfo.CodecPrivate) > 0 {
		result.CodecPrivate = make([]byte, len(trackInfo.CodecPrivate))
		copy(result.CodecPrivate, trackInfo.CodecPrivate)
	}

	if len(trackInfo.CompMethodPrivate) > 0 {
		result.CompMethodPrivate = make([]byte, len(trackInfo.CompMethodPrivate))
		copy(result.CompMethodPrivate, trackInfo.CompMethodPrivate)
	}

	return result, nil
}

func (d *Demuxer) GetFileInfo() (*SegmentInfo, error) {
	if d.closed {
		return nil, fmt.Errorf("demuxer is closed")
	}

	if d.parser.segmentInfo == nil {
		return nil, fmt.Errorf("no segment info found")
	}

	result := &SegmentInfo{}
	*result = *d.parser.segmentInfo

	return result, nil
}

func (d *Demuxer) GetAttachments() []*Attachment {
	if d.closed {
		return []*Attachment{}
	}

	result := make([]*Attachment, len(d.parser.attachments))
	for i, attachment := range d.parser.attachments {
		result[i] = &Attachment{}
		*result[i] = *attachment
	}

	return result
}

func (d *Demuxer) GetChapters() []*Chapter {
	if d.closed {
		return []*Chapter{}
	}

	result := make([]*Chapter, len(d.parser.chapters))
	for i, chapter := range d.parser.chapters {
		result[i] = d.copyChapter(chapter)
	}

	return result
}

func (d *Demuxer) copyChapter(src *Chapter) *Chapter {
	dst := &Chapter{}
	*dst = *src

	if len(src.Tracks) > 0 {
		dst.Tracks = make([]uint64, len(src.Tracks))
		copy(dst.Tracks, src.Tracks)
	}

	if len(src.Display) > 0 {
		dst.Display = make([]ChapterDisplay, len(src.Display))
		copy(dst.Display, src.Display)
	}

	if len(src.Children) > 0 {
		dst.Children = make([]*Chapter, len(src.Children))
		for i, child := range src.Children {
			dst.Children[i] = d.copyChapter(child)
		}
	}

	if len(src.Process) > 0 {
		dst.Process = make([]ChapterProcess, len(src.Process))
		for i, process := range src.Process {
			dst.Process[i] = ChapterProcess{
				CodecID: process.CodecID,
			}
			if len(process.CodecPrivate) > 0 {
				dst.Process[i].CodecPrivate = make([]byte, len(process.CodecPrivate))
				copy(dst.Process[i].CodecPrivate, process.CodecPrivate)
			}
			if len(process.Commands) > 0 {
				dst.Process[i].Commands = make([]ChapterCommand, len(process.Commands))
				for j, command := range process.Commands {
					dst.Process[i].Commands[j] = ChapterCommand{
						Time: command.Time,
					}
					if len(command.Command) > 0 {
						dst.Process[i].Commands[j].Command = make([]byte, len(command.Command))
						copy(dst.Process[i].Commands[j].Command, command.Command)
					}
				}
			}
		}
	}

	return dst
}

func (d *Demuxer) GetTags() []*Tag {
	if d.closed {
		return []*Tag{}
	}

	result := make([]*Tag, len(d.parser.tags))
	for i, tag := range d.parser.tags {
		result[i] = &Tag{}
		*result[i] = *tag

		if len(tag.Targets) > 0 {
			result[i].Targets = make([]Target, len(tag.Targets))
			copy(result[i].Targets, tag.Targets)
		}

		if len(tag.SimpleTags) > 0 {
			result[i].SimpleTags = make([]SimpleTag, len(tag.SimpleTags))
			copy(result[i].SimpleTags, tag.SimpleTags)
		}
	}

	return result
}

func (d *Demuxer) GetCues() []*Cue {
	if d.closed {
		return []*Cue{}
	}

	result := make([]*Cue, len(d.parser.cues))
	for i, cue := range d.parser.cues {
		result[i] = &Cue{}
		*result[i] = *cue
	}

	return result
}

func (d *Demuxer) GetSegment() uint64 {
	if d.closed {
		return 0
	}
	return d.parser.segmentPos
}

func (d *Demuxer) GetSegmentTop() uint64 {
	if d.closed {
		return 0
	}
	return d.parser.segmentPos + d.parser.segmentSize
}

func (d *Demuxer) GetCuesPos() uint64 {
	if d.closed || len(d.parser.cues) == 0 {
		return 0
	}

	return d.parser.cues[0].Position
}

func (d *Demuxer) GetCuesTopPos() uint64 {
	if d.closed || len(d.parser.cues) == 0 {
		return 0
	}

	lastCue := d.parser.cues[len(d.parser.cues)-1]
	return lastCue.Position + lastCue.Duration
}

func (d *Demuxer) Seek(timecode uint64, flags uint32) {
	if d.closed {
		return
	}

	_ = d.packetReader.Seek(timecode, flags)
}

func (d *Demuxer) SeekCueAware(timecode uint64, flags uint32) {
	if d.closed {
		return
	}

	_ = d.packetReader.Seek(timecode, flags)
}

func (d *Demuxer) SkipToKeyframe() {
	if d.closed {
		return
	}

	_ = d.packetReader.SkipToKeyframe()
}

func (d *Demuxer) GetLowestQTimecode() uint64 {
	if d.closed {
		return 0
	}

	return d.packetReader.GetLowestQTimecode()
}

func (d *Demuxer) SetTrackMask(mask uint64) {
	if d.closed {
		return
	}

	d.packetReader.SetTrackMask(mask)
}

func (d *Demuxer) ReadPacketMask(mask uint64) (*Packet, error) {
	if d.closed {
		return nil, fmt.Errorf("demuxer is closed")
	}

	return d.packetReader.ReadPacketWithMask(mask)
}

func (d *Demuxer) ReadPacket() (*Packet, error) {
	if d.closed {
		return nil, fmt.Errorf("demuxer is closed")
	}

	return d.packetReader.ReadPacket()
}
