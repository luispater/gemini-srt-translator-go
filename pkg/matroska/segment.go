package matroska

import (
	"io"
)

func (p *Parser) parseSegmentInfo(element *EBMLElement) error {
	reader := NewEBMLReader(&bytesReader{data: element.Data})

	info := &SegmentInfo{
		TimecodeScale: 1000000,
	}

	for reader.Position() < uint64(len(element.Data)) {
		child, err := reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		switch child.ID {
		case TimecodeScaleID:
			scale, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			info.TimecodeScale = scale
		case DurationID:
			duration, errReadFloat := child.ReadFloat()
			if errReadFloat != nil {
				return errReadFloat
			}
			info.Duration = uint64(duration)
		case DateUTCID:
			date, errReadInt := child.ReadInt()
			if errReadInt != nil {
				return errReadInt
			}
			info.DateUTC = date
			info.DateUTCValid = true
		case TitleID:
			info.Title = child.ReadString()
		case MuxingAppID:
			info.MuxingApp = child.ReadString()
		case WritingAppID:
			info.WritingApp = child.ReadString()
		case SegmentUIDID:
			uid := child.ReadBytes()
			if len(uid) <= 16 {
				copy(info.UID[:], uid)
			}
		case SegmentFilenameID:
			info.Filename = child.ReadString()
		case PrevUIDID:
			uid := child.ReadBytes()
			if len(uid) <= 16 {
				copy(info.PrevUID[:], uid)
			}
		case PrevFilenameID:
			info.PrevFilename = child.ReadString()
		case NextUIDID:
			uid := child.ReadBytes()
			if len(uid) <= 16 {
				copy(info.NextUID[:], uid)
			}
		case NextFilenameID:
			info.NextFilename = child.ReadString()
		}
	}

	p.segmentInfo = info
	return nil
}

func (p *Parser) parseTracks(element *EBMLElement) error {
	reader := NewEBMLReader(&bytesReader{data: element.Data})

	for reader.Position() < uint64(len(element.Data)) {
		child, err := reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if child.ID == TrackEntryID {
			track, errParseTrackEntry := p.parseTrackEntry(child)
			if errParseTrackEntry != nil {
				return errParseTrackEntry
			}
			p.tracks = append(p.tracks, track)
		}
	}

	return nil
}

func (p *Parser) parseTrackEntry(element *EBMLElement) (*TrackInfo, error) {
	reader := NewEBMLReader(&bytesReader{data: element.Data})

	track := &TrackInfo{
		Enabled:       true,
		Lacing:        true,
		Language:      "eng",
		TimecodeScale: 1.0,
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
		case TrackNumberID:
			num, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return nil, errReadUint
			}
			track.Number = uint8(num)
		case TrackUIDID:
			uid, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return nil, errReadUint
			}
			track.UID = uid
		case TrackTypeID:
			trackType, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return nil, errReadUint
			}
			track.Type = uint8(trackType)
		case FlagEnabledID:
			enabled, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return nil, errReadUint
			}
			track.Enabled = enabled != 0
		case FlagDefaultID:
			def, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return nil, errReadUint
			}
			track.Default = def != 0
		case FlagForcedID:
			forced, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return nil, errReadUint
			}
			track.Forced = forced != 0
		case FlagLacingID:
			lacing, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return nil, errReadUint
			}
			track.Lacing = lacing != 0
		case MinCacheID:
			cache, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return nil, errReadUint
			}
			track.MinCache = cache
		case MaxCacheID:
			cache, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return nil, errReadUint
			}
			track.MaxCache = cache
		case DefaultDurationID:
			duration, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return nil, errReadUint
			}
			track.DefaultDuration = duration
		case CodecDelayID:
			delay, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return nil, errReadUint
			}
			track.CodecDelay = delay
		case SeekPreRollID:
			preroll, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return nil, errReadUint
			}
			track.SeekPreRoll = preroll
		case TrackTimecodeScaleID:
			scale, errReadUint := child.ReadFloat()
			if errReadUint != nil {
				return nil, errReadUint
			}
			track.TimecodeScale = scale
		case MaxBlockAdditionIDID:
			maxID, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return nil, errReadUint
			}
			track.MaxBlockAdditionID = uint32(maxID)
		case NameID:
			track.Name = child.ReadString()
		case LanguageID:
			lang := child.ReadString()
			if len(lang) >= 3 {
				track.Language = lang[:3]
			} else {
				track.Language = lang
			}
		case CodecIDID:
			track.CodecID = child.ReadString()
		case CodecPrivateID:
			track.CodecPrivate = child.ReadBytes()
		case VideoID:
			if err = p.parseVideoInfo(child, track); err != nil {
				return nil, err
			}
		case AudioID:
			if err = p.parseAudioInfo(child, track); err != nil {
				return nil, err
			}
		case ContentEncodingsID:
			if err = p.parseContentEncodings(child, track); err != nil {
				return nil, err
			}
		}
	}

	return track, nil
}

func (p *Parser) parseVideoInfo(element *EBMLElement, track *TrackInfo) error {
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
		case FlagInterlacedID:
			interlaced, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.Interlaced = interlaced != 0
		case StereoModeID:
			mode, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.StereoMode = uint8(mode)
		case PixelWidthID:
			width, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.PixelWidth = uint32(width)
		case PixelHeightID:
			height, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.PixelHeight = uint32(height)
		case DisplayWidthID:
			width, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.DisplayWidth = uint32(width)
		case DisplayHeightID:
			height, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.DisplayHeight = uint32(height)
		case DisplayUnitID:
			unit, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.DisplayUnit = uint8(unit)
		case AspectRatioTypeID:
			ratioType, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.AspectRatioType = uint8(ratioType)
		case PixelCropLeftID:
			crop, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.CropL = uint32(crop)
		case PixelCropTopID:
			crop, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.CropT = uint32(crop)
		case PixelCropRightID:
			crop, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.CropR = uint32(crop)
		case PixelCropBottomID:
			crop, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.CropB = uint32(crop)
		case ColourSpaceID:
			colorSpace, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.ColourSpace = uint32(colorSpace)
		case GammaValueID:
			gamma, errReadUint := child.ReadFloat()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.GammaValue = gamma
		case ColorID:
			if err = p.parseColorInfo(child, track); err != nil {
				return err
			}
		}
	}

	if track.Video.DisplayWidth == 0 {
		track.Video.DisplayWidth = track.Video.PixelWidth
	}
	if track.Video.DisplayHeight == 0 {
		track.Video.DisplayHeight = track.Video.PixelHeight
	}

	return nil
}

func (p *Parser) parseColorInfo(element *EBMLElement, track *TrackInfo) error {
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
		case MatrixCoefficientsID:
			coeff, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.Colour.MatrixCoefficients = uint32(coeff)
		case BitsPerChannelID:
			bits, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.Colour.BitsPerChannel = uint32(bits)
		case ChromaSubsamplingHorzID:
			chroma, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.Colour.ChromaSubsamplingHorz = uint32(chroma)
		case ChromaSubsamplingVertID:
			chroma, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.Colour.ChromaSubsamplingVert = uint32(chroma)
		case CbSubsamplingHorzID:
			cb, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.Colour.CbSubsamplingHorz = uint32(cb)
		case CbSubsamplingVertID:
			cb, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.Colour.CbSubsamplingVert = uint32(cb)
		case ChromaSitingHorzID:
			siting, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.Colour.ChromaSitingHorz = uint32(siting)
		case ChromaSitingVertID:
			siting, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.Colour.ChromaSitingVert = uint32(siting)
		case RangeID:
			colorRange, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.Colour.Range = uint32(colorRange)
		case TransferCharacteristicsID:
			transfer, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.Colour.TransferCharacteristics = uint32(transfer)
		case PrimariesID:
			primaries, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.Colour.Primaries = uint32(primaries)
		case MaxCLLID:
			maxCLL, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.Colour.MaxCLL = uint32(maxCLL)
		case MaxFALLID:
			maxFALL, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Video.Colour.MaxFALL = uint32(maxFALL)
		case MasteringMetadataID:
			if err = p.parseMasteringMetadata(child, track); err != nil {
				return err
			}
		}
	}

	return nil
}

func (p *Parser) parseMasteringMetadata(element *EBMLElement, track *TrackInfo) error {
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
		case PrimaryRChromaticityXID:
			val, errReadFloat := child.ReadFloat()
			if errReadFloat != nil {
				return errReadFloat
			}
			track.Video.Colour.MasteringMetadata.PrimaryRChromaticityX = float32(val)
		case PrimaryRChromaticityYID:
			val, errReadFloat := child.ReadFloat()
			if errReadFloat != nil {
				return errReadFloat
			}
			track.Video.Colour.MasteringMetadata.PrimaryRChromaticityY = float32(val)
		case PrimaryGChromaticityXID:
			val, errReadFloat := child.ReadFloat()
			if errReadFloat != nil {
				return errReadFloat
			}
			track.Video.Colour.MasteringMetadata.PrimaryGChromaticityX = float32(val)
		case PrimaryGChromaticityYID:
			val, errReadFloat := child.ReadFloat()
			if errReadFloat != nil {
				return errReadFloat
			}
			track.Video.Colour.MasteringMetadata.PrimaryGChromaticityY = float32(val)
		case PrimaryBChromaticityXID:
			val, errReadFloat := child.ReadFloat()
			if errReadFloat != nil {
				return errReadFloat
			}
			track.Video.Colour.MasteringMetadata.PrimaryBChromaticityX = float32(val)
		case PrimaryBChromaticityYID:
			val, errReadFloat := child.ReadFloat()
			if errReadFloat != nil {
				return errReadFloat
			}
			track.Video.Colour.MasteringMetadata.PrimaryBChromaticityY = float32(val)
		case WhitePointChromaticityXID:
			val, errReadFloat := child.ReadFloat()
			if errReadFloat != nil {
				return errReadFloat
			}
			track.Video.Colour.MasteringMetadata.WhitePointChromaticityX = float32(val)
		case WhitePointChromaticityYID:
			val, errReadFloat := child.ReadFloat()
			if errReadFloat != nil {
				return errReadFloat
			}
			track.Video.Colour.MasteringMetadata.WhitePointChromaticityY = float32(val)
		case LuminanceMaxID:
			val, errReadFloat := child.ReadFloat()
			if errReadFloat != nil {
				return errReadFloat
			}
			track.Video.Colour.MasteringMetadata.LuminanceMax = float32(val)
		case LuminanceMinID:
			val, errReadFloat := child.ReadFloat()
			if errReadFloat != nil {
				return errReadFloat
			}
			track.Video.Colour.MasteringMetadata.LuminanceMin = float32(val)
		}
	}

	return nil
}

func (p *Parser) parseAudioInfo(element *EBMLElement, track *TrackInfo) error {
	reader := NewEBMLReader(&bytesReader{data: element.Data})

	track.Audio.Channels = 1

	for reader.Position() < uint64(len(element.Data)) {
		child, err := reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		switch child.ID {
		case SamplingFrequencyID:
			freq, errReadFloat := child.ReadFloat()
			if errReadFloat != nil {
				return errReadFloat
			}
			track.Audio.SamplingFreq = freq
		case OutputSamplingFrequencyID:
			freq, errReadFloat := child.ReadFloat()
			if errReadFloat != nil {
				return errReadFloat
			}
			track.Audio.OutputSamplingFreq = freq
		case ChannelsID:
			channels, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Audio.Channels = uint8(channels)
		case BitDepthID:
			depth, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.Audio.BitDepth = uint8(depth)
		}
	}

	if track.Audio.OutputSamplingFreq == 0 {
		track.Audio.OutputSamplingFreq = track.Audio.SamplingFreq
	}

	return nil
}

func (p *Parser) parseContentEncodings(element *EBMLElement, track *TrackInfo) error {
	reader := NewEBMLReader(&bytesReader{data: element.Data})

	for reader.Position() < uint64(len(element.Data)) {
		child, err := reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if child.ID == ContentEncodingID {
			if err = p.parseContentEncoding(child, track); err != nil {
				return err
			}
		}
	}

	return nil
}

func (p *Parser) parseContentEncoding(element *EBMLElement, track *TrackInfo) error {
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
		case ContentCompressionID:
			if err = p.parseContentCompression(child, track); err != nil {
				return err
			}
		}
	}

	return nil
}

func (p *Parser) parseContentCompression(element *EBMLElement, track *TrackInfo) error {
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
		case ContentCompAlgoID:
			algo, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return errReadUint
			}
			track.CompMethod = uint32(algo)
			track.CompEnabled = true
		case ContentCompSettingsID:
			track.CompMethodPrivate = child.ReadBytes()
		}
	}

	return nil
}
