package matroska

import (
	"io"
)

func (p *Parser) parseAttachments(element *EBMLElement) error {
	reader := NewEBMLReader(&bytesReader{data: element.Data})

	for reader.Position() < uint64(len(element.Data)) {
		child, err := reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if child.ID == AttachedFileID {
			attachment, errParseAttachedFile := p.parseAttachedFile(child)
			if errParseAttachedFile != nil {
				return errParseAttachedFile
			}
			p.attachments = append(p.attachments, attachment)
		}
	}

	return nil
}

func (p *Parser) parseAttachedFile(element *EBMLElement) (*Attachment, error) {
	reader := NewEBMLReader(&bytesReader{data: element.Data})

	attachment := &Attachment{
		Position: element.Offset,
		Length:   element.Size,
	}

	for reader.Position() < uint64(len(element.Data)) {
		child, err := reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		switch child.ID {
		case FileNameID:
			attachment.Name = child.ReadString()
		case FileDescriptionID:
			attachment.Description = child.ReadString()
		case FileMimeTypeID:
			attachment.MimeType = child.ReadString()
		case FileUIDID:
			uid, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return nil, errReadUint
			}
			attachment.UID = uid
		case FileDataID:
			attachment.Position = child.Offset
			attachment.Length = uint64(len(child.Data))
		}
	}

	return attachment, nil
}

func (p *Parser) parseChapters(element *EBMLElement) error {
	reader := NewEBMLReader(&bytesReader{data: element.Data})

	for reader.Position() < uint64(len(element.Data)) {
		child, err := reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if child.ID == EditionEntryID {
			chapters, errParseEditionEntry := p.parseEditionEntry(child)
			if errParseEditionEntry != nil {
				return errParseEditionEntry
			}
			p.chapters = append(p.chapters, chapters...)
		}
	}

	return nil
}

func (p *Parser) parseEditionEntry(element *EBMLElement) ([]*Chapter, error) {
	reader := NewEBMLReader(&bytesReader{data: element.Data})

	var chapters []*Chapter

	for reader.Position() < uint64(len(element.Data)) {
		child, err := reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		if child.ID == ChapterAtomID {
			chapter, errParseChapterAtom := p.parseChapterAtom(child)
			if errParseChapterAtom != nil {
				return nil, errParseChapterAtom
			}
			chapters = append(chapters, chapter)
		}
	}

	return chapters, nil
}

func (p *Parser) parseChapterAtom(element *EBMLElement) (*Chapter, error) {
	reader := NewEBMLReader(&bytesReader{data: element.Data})

	chapter := &Chapter{
		Enabled: true,
	}

	for reader.Position() < uint64(len(element.Data)) {
		child, err := reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		switch child.ID {
		case ChapterUIDID:
			uid, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return nil, errReadUint
			}
			chapter.UID = uid
		case ChapterTimeStartID:
			start, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return nil, errReadUint
			}
			chapter.Start = start
		case ChapterTimeEndID:
			end, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return nil, errReadUint
			}
			chapter.End = end
		case ChapterFlagHiddenID:
			hidden, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return nil, errReadUint
			}
			chapter.Hidden = hidden != 0
		case ChapterFlagEnabledID:
			enabled, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return nil, errReadUint
			}
			chapter.Enabled = enabled != 0
		case ChapterSegmentUIDID:
			uid := child.ReadBytes()
			if len(uid) <= 16 {
				copy(chapter.SegmentUID[:], uid)
			}
		case ChapterDisplayID:
			display, errParseChapterDisplay := p.parseChapterDisplay(child)
			if errParseChapterDisplay != nil {
				return nil, errParseChapterDisplay
			}
			chapter.Display = append(chapter.Display, display)
		case ChapterAtomID:
			childChapter, errParseChapterAtom := p.parseChapterAtom(child)
			if errParseChapterAtom != nil {
				return nil, errParseChapterAtom
			}
			chapter.Children = append(chapter.Children, childChapter)
		case ChapterProcessID:
			process, errParseChapterProcess := p.parseChapterProcess(child)
			if errParseChapterProcess != nil {
				return nil, errParseChapterProcess
			}
			chapter.Process = append(chapter.Process, process)
		}
	}

	return chapter, nil
}

func (p *Parser) parseChapterDisplay(element *EBMLElement) (ChapterDisplay, error) {
	reader := NewEBMLReader(&bytesReader{data: element.Data})

	var display ChapterDisplay

	for reader.Position() < uint64(len(element.Data)) {
		child, err := reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return display, err
		}

		switch child.ID {
		case ChapStringID:
			display.String = child.ReadString()
		case ChapLanguageID:
			lang := child.ReadString()
			if len(lang) >= 3 {
				display.Language = lang[:3]
			} else {
				display.Language = lang
			}
		case ChapCountryID:
			country := child.ReadString()
			if len(country) >= 3 {
				display.Country = country[:3]
			} else {
				display.Country = country
			}
		}
	}

	return display, nil
}

func (p *Parser) parseChapterProcess(element *EBMLElement) (ChapterProcess, error) {
	reader := NewEBMLReader(&bytesReader{data: element.Data})

	var process ChapterProcess

	for reader.Position() < uint64(len(element.Data)) {
		child, err := reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return process, err
		}

		switch child.ID {
		case ChapterProcessCodecIDID:
			codecID, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return process, errReadUint
			}
			process.CodecID = uint32(codecID)
		case ChapterProcessPrivateID:
			process.CodecPrivate = child.ReadBytes()
		case ChapterProcessCommandID:
			command, errParseChapterCommand := p.parseChapterCommand(child)
			if errParseChapterCommand != nil {
				return process, errParseChapterCommand
			}
			process.Commands = append(process.Commands, command)
		}
	}

	return process, nil
}

func (p *Parser) parseChapterCommand(element *EBMLElement) (ChapterCommand, error) {
	reader := NewEBMLReader(&bytesReader{data: element.Data})

	var command ChapterCommand

	for reader.Position() < uint64(len(element.Data)) {
		child, err := reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return command, err
		}

		switch child.ID {
		case ChapterProcessTimeID:
			time, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return command, errReadUint
			}
			command.Time = uint32(time)
		case ChapterProcessDataID:
			command.Command = child.ReadBytes()
		}
	}

	return command, nil
}

func (p *Parser) parseTags(element *EBMLElement) error {
	reader := NewEBMLReader(&bytesReader{data: element.Data})

	for reader.Position() < uint64(len(element.Data)) {
		child, err := reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if child.ID == TagID {
			tag, errParseTag := p.parseTag(child)
			if errParseTag != nil {
				return errParseTag
			}
			p.tags = append(p.tags, tag)
		}
	}

	return nil
}

func (p *Parser) parseTag(element *EBMLElement) (*Tag, error) {
	reader := NewEBMLReader(&bytesReader{data: element.Data})

	tag := &Tag{}

	for reader.Position() < uint64(len(element.Data)) {
		child, err := reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		switch child.ID {
		case TargetsID:
			targets, errParseTargets := p.parseTargets(child)
			if errParseTargets != nil {
				return nil, errParseTargets
			}
			tag.Targets = append(tag.Targets, targets...)
		case SimpleTagID:
			simpleTag, errParseSimpleTag := p.parseSimpleTag(child)
			if errParseSimpleTag != nil {
				return nil, errParseSimpleTag
			}
			tag.SimpleTags = append(tag.SimpleTags, simpleTag)
		}
	}

	return tag, nil
}

func (p *Parser) parseTargets(element *EBMLElement) ([]Target, error) {
	reader := NewEBMLReader(&bytesReader{data: element.Data})

	var targets []Target
	target := Target{}

	for reader.Position() < uint64(len(element.Data)) {
		child, err := reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		switch child.ID {
		case TargetTypeValueID:
			typeValue, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return nil, errReadUint
			}
			target.Type = uint32(typeValue)
		case TagTrackUIDID:
			uid, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return nil, errReadUint
			}
			target.UID = uid
		}
	}

	targets = append(targets, target)
	return targets, nil
}

func (p *Parser) parseSimpleTag(element *EBMLElement) (SimpleTag, error) {
	reader := NewEBMLReader(&bytesReader{data: element.Data})

	var simpleTag SimpleTag
	simpleTag.Language = "und"

	for reader.Position() < uint64(len(element.Data)) {
		child, err := reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return simpleTag, err
		}

		switch child.ID {
		case TagNameID:
			simpleTag.Name = child.ReadString()
		case TagStringID:
			simpleTag.Value = child.ReadString()
		case TagLanguageID:
			lang := child.ReadString()
			if len(lang) >= 3 {
				simpleTag.Language = lang[:3]
			} else {
				simpleTag.Language = lang
			}
		case TagDefaultID:
			def, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return simpleTag, errReadUint
			}
			simpleTag.Default = def != 0
		}
	}

	return simpleTag, nil
}

func (p *Parser) parseCues(element *EBMLElement) error {
	reader := NewEBMLReader(&bytesReader{data: element.Data})

	for reader.Position() < uint64(len(element.Data)) {
		child, err := reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if child.ID == CuePointID {
			cue, errParseCuePoint := p.parseCuePoint(child)
			if errParseCuePoint != nil {
				return errParseCuePoint
			}
			p.cues = append(p.cues, cue)
		}
	}

	return nil
}

func (p *Parser) parseCuePoint(element *EBMLElement) (*Cue, error) {
	reader := NewEBMLReader(&bytesReader{data: element.Data})

	cue := &Cue{}

	for reader.Position() < uint64(len(element.Data)) {
		child, err := reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		switch child.ID {
		case CueTimeID:
			time, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return nil, errReadUint
			}
			cue.Time = time
		case CueDurationID:
			duration, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return nil, errReadUint
			}
			cue.Duration = duration
		case CueTrackPositionsID:
			if err = p.parseCueTrackPositions(child, cue); err != nil {
				return nil, err
			}
		}
	}

	return cue, nil
}

func (p *Parser) parseCueTrackPositions(element *EBMLElement, cue *Cue) error {
	reader := NewEBMLReader(&bytesReader{data: element.Data})

	for reader.Position() < uint64(len(element.Data)) {
		child, err := reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		switch child.ID {
		case CueTrackID:
			track, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			cue.Track = uint8(track)
		case CueClusterPositionID:
			pos, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			cue.Position = pos
		case CueRelativePositionID:
			relPos, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			cue.RelativePosition = relPos
		case CueBlockNumberID:
			block, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			cue.Block = block
		}
	}

	return nil
}

func (p *Parser) parseClusterInfo(element *EBMLElement) (*ClusterInfo, error) {
	cluster := &ClusterInfo{
		Position: element.Offset,
		Size:     element.Size,
	}

	// For Cluster elements, we don't read the data into memory
	// Just return basic info
	return cluster, nil
}
