package matroska

const (
	EBMLHeaderID         = 0x1A45DFA3
	EBMLVersionID        = 0x4286
	EBMLReadVersionID    = 0x42F7
	EBMLMaxIDLengthID    = 0x42F2
	EBMLMaxSizeLengthID  = 0x42F3
	DocTypeID            = 0x4282
	DocTypeVersionID     = 0x4287
	DocTypeReadVersionID = 0x4285

	SegmentID     = 0x18538067
	SeekHeadID    = 0x114D9B74
	SegmentInfoID = 0x1549A966
	TracksID      = 0x1654AE6B
	CuesID        = 0x1C53BB6B
	AttachmentsID = 0x1941A469
	ChaptersID    = 0x1043A770
	TagsID        = 0x1254C367
	ClusterID     = 0x1F43B675

	SeekID          = 0x4DBB
	SeekIDElementID = 0x53AB
	SeekPositionID  = 0x53AC

	TimecodeScaleID   = 0x2AD7B1
	DurationID        = 0x4489
	DateUTCID         = 0x4461
	TitleID           = 0x7BA9
	MuxingAppID       = 0x4D80
	WritingAppID      = 0x5741
	SegmentUIDID      = 0x73A4
	SegmentFilenameID = 0x7384
	PrevUIDID         = 0x3CB923
	PrevFilenameID    = 0x3C83AB
	NextUIDID         = 0x3EB923
	NextFilenameID    = 0x3E83BB

	TrackEntryID         = 0xAE
	TrackNumberID        = 0xD7
	TrackUIDID           = 0x73C5
	TrackTypeID          = 0x83
	FlagEnabledID        = 0xB9
	FlagDefaultID        = 0x88
	FlagForcedID         = 0x55AA
	FlagLacingID         = 0x9C
	MinCacheID           = 0x6DE7
	MaxCacheID           = 0x6DF8
	DefaultDurationID    = 0x23E383
	MaxBlockAdditionIDID = 0x55EE
	NameID               = 0x536E
	LanguageID           = 0x22B59C
	CodecIDID            = 0x86
	CodecPrivateID       = 0x63A2
	CodecNameID          = 0x258688
	CodecDelayID         = 0x56AA
	SeekPreRollID        = 0x56BB
	TrackTimecodeScaleID = 0x23314F

	VideoID           = 0xE0
	FlagInterlacedID  = 0x9A
	StereoModeID      = 0x53B8
	AlphaModeID       = 0x53C0
	PixelWidthID      = 0xB0
	PixelHeightID     = 0xBA
	PixelCropBottomID = 0x54AA
	PixelCropTopID    = 0x54BB
	PixelCropLeftID   = 0x54CC
	PixelCropRightID  = 0x54DD
	DisplayWidthID    = 0x54B0
	DisplayHeightID   = 0x54BA
	DisplayUnitID     = 0x54B2
	AspectRatioTypeID = 0x54B3
	ColourSpaceID     = 0x2EB524
	GammaValueID      = 0x2FB523

	ColorID                   = 0x55B0
	MatrixCoefficientsID      = 0x55B1
	BitsPerChannelID          = 0x55B2
	ChromaSubsamplingHorzID   = 0x55B3
	ChromaSubsamplingVertID   = 0x55B4
	CbSubsamplingHorzID       = 0x55B5
	CbSubsamplingVertID       = 0x55B6
	ChromaSitingHorzID        = 0x55B7
	ChromaSitingVertID        = 0x55B8
	RangeID                   = 0x55B9
	TransferCharacteristicsID = 0x55BA
	PrimariesID               = 0x55BB
	MaxCLLID                  = 0x55BC
	MaxFALLID                 = 0x55BD

	MasteringMetadataID       = 0x55D0
	PrimaryRChromaticityXID   = 0x55D1
	PrimaryRChromaticityYID   = 0x55D2
	PrimaryGChromaticityXID   = 0x55D3
	PrimaryGChromaticityYID   = 0x55D4
	PrimaryBChromaticityXID   = 0x55D5
	PrimaryBChromaticityYID   = 0x55D6
	WhitePointChromaticityXID = 0x55D7
	WhitePointChromaticityYID = 0x55D8
	LuminanceMaxID            = 0x55D9
	LuminanceMinID            = 0x55DA

	AudioID                   = 0xE1
	SamplingFrequencyID       = 0xB5
	OutputSamplingFrequencyID = 0x78B5
	ChannelsID                = 0x9F
	BitDepthID                = 0x6264

	ContentEncodingsID     = 0x6D80
	ContentEncodingID      = 0x6240
	ContentEncodingOrderID = 0x5031
	ContentEncodingScopeID = 0x5032
	ContentEncodingTypeID  = 0x5033
	ContentCompressionID   = 0x5034
	ContentCompAlgoID      = 0x4254
	ContentCompSettingsID  = 0x4255

	TimecodeID          = 0xE7
	SilentTracksID      = 0x5854
	PositionID          = 0xA7
	PrevSizeID          = 0xAB
	SimpleBlockID       = 0xA3
	BlockGroupID        = 0xA0
	BlockID             = 0xA1
	BlockDurationID     = 0x9B
	ReferencePriorityID = 0xFA
	ReferenceBlockID    = 0xFB
	CodecStateID        = 0xA4
	DiscardPaddingID    = 0x75A2
	SlicesID            = 0x8E
	TimeSliceID         = 0xE8
	LaceNumberID        = 0xCC
	FrameNumberID       = 0xCD
	BlockAdditionIDID   = 0xEE
	DelayID             = 0xCE
	SliceDurationID     = 0xCF
	ReferenceFrameID    = 0xC8
	ReferenceOffsetID   = 0xC9
	ReferenceTimecodeID = 0xCA

	CuePointID            = 0xBB
	CueTimeID             = 0xB3
	CueTrackPositionsID   = 0xB7
	CueTrackID            = 0xF7
	CueClusterPositionID  = 0xF1
	CueRelativePositionID = 0xF0
	CueDurationID         = 0xB2
	CueBlockNumberID      = 0x5378
	CueCodecStateID       = 0xEA
	CueReferenceID        = 0xDB
	CueRefTimeID          = 0x96

	AttachedFileID    = 0x61A7
	FileDescriptionID = 0x467E
	FileNameID        = 0x466E
	FileMimeTypeID    = 0x4660
	FileDataID        = 0x465C
	FileUIDID         = 0x46AE

	EditionEntryID          = 0x45B9
	EditionUIDID            = 0x45BC
	EditionFlagHiddenID     = 0x45BD
	EditionFlagDefaultID    = 0x45DB
	EditionFlagOrderedID    = 0x45DD
	ChapterAtomID           = 0xB6
	ChapterUIDID            = 0x73C4
	ChapterStringUIDID      = 0x5654
	ChapterTimeStartID      = 0x91
	ChapterTimeEndID        = 0x92
	ChapterFlagHiddenID     = 0x98
	ChapterFlagEnabledID    = 0x4598
	ChapterSegmentUIDID     = 0x6E67
	ChapterDisplayID        = 0x80
	ChapStringID            = 0x85
	ChapLanguageID          = 0x437C
	ChapCountryID           = 0x437E
	ChapterProcessID        = 0x6944
	ChapterProcessCodecIDID = 0x6955
	ChapterProcessPrivateID = 0x450D
	ChapterProcessCommandID = 0x6911
	ChapterProcessTimeID    = 0x6922
	ChapterProcessDataID    = 0x6933

	TagID              = 0x7373
	TargetsID          = 0x63C0
	TargetTypeValueID  = 0x68CA
	TargetTypeID       = 0x63CA
	TagTrackUIDID      = 0x63C5
	TagEditionUIDID    = 0x63C9
	TagChapterUIDID    = 0x63C4
	TagAttachmentUIDID = 0x63C6
	SimpleTagID        = 0x67C8
	TagNameID          = 0x45A3
	TagLanguageID      = 0x447A
	TagDefaultID       = 0x4484
	TagStringID        = 0x4487
	TagBinaryID        = 0x4485
)

var ElementNames = map[uint32]string{
	EBMLHeaderID:         "EBML",
	EBMLVersionID:        "EBMLVersion",
	EBMLReadVersionID:    "EBMLReadVersion",
	EBMLMaxIDLengthID:    "EBMLMaxIDLength",
	EBMLMaxSizeLengthID:  "EBMLMaxSizeLength",
	DocTypeID:            "DocType",
	DocTypeVersionID:     "DocTypeVersion",
	DocTypeReadVersionID: "DocTypeReadVersion",

	SegmentID:     "Segment",
	SeekHeadID:    "SeekHead",
	SegmentInfoID: "Info",
	TracksID:      "Tracks",
	CuesID:        "Cues",
	AttachmentsID: "Attachments",
	ChaptersID:    "Chapters",
	TagsID:        "Tags",
	ClusterID:     "Cluster",

	TimecodeScaleID: "TimecodeScale",
	DurationID:      "Duration",
	DateUTCID:       "DateUTC",
	TitleID:         "Title",
	MuxingAppID:     "MuxingApp",
	WritingAppID:    "WritingApp",

	TrackEntryID:   "TrackEntry",
	TrackNumberID:  "TrackNumber",
	TrackUIDID:     "TrackUID",
	TrackTypeID:    "TrackType",
	CodecIDID:      "CodecID",
	CodecPrivateID: "CodecPrivate",
	NameID:         "Name",
	LanguageID:     "Language",

	VideoID:         "Video",
	PixelWidthID:    "PixelWidth",
	PixelHeightID:   "PixelHeight",
	DisplayWidthID:  "DisplayWidth",
	DisplayHeightID: "DisplayHeight",

	AudioID:             "Audio",
	SamplingFrequencyID: "SamplingFrequency",
	ChannelsID:          "Channels",
	BitDepthID:          "BitDepth",

	TimecodeID:    "Timecode",
	SimpleBlockID: "SimpleBlock",
	BlockGroupID:  "BlockGroup",
	BlockID:       "Block",

	CuePointID:           "CuePoint",
	CueTimeID:            "CueTime",
	CueTrackPositionsID:  "CueTrackPositions",
	CueTrackID:           "CueTrack",
	CueClusterPositionID: "CueClusterPosition",
}
