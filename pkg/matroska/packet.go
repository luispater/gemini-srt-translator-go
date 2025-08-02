package matroska

import (
	"encoding/binary"
	"errors"
	"io"
)

type PacketReader struct {
	parser         *Parser
	trackMask      uint64
	currentCluster *ClusterInfo
	clusterReader  *EBMLReader
	clusterData    []byte
	seekMask       uint64
	lowestTimecode uint64
}

func NewPacketReader(parser *Parser) *PacketReader {
	return &PacketReader{
		parser: parser,
	}
}

func (pr *PacketReader) SetTrackMask(mask uint64) {
	pr.trackMask = mask
}

func (pr *PacketReader) ReadPacket() (*Packet, error) {
	return pr.ReadPacketWithMask(pr.trackMask)
}

func (pr *PacketReader) ReadPacketWithMask(mask uint64) (*Packet, error) {
	for {
		if pr.clusterReader == nil {
			if err := pr.loadNextCluster(); err != nil {
				return nil, err
			}
		}

		packet, err := pr.readPacketFromCluster(mask)
		if err != nil {
			if err == io.EOF {
				pr.clusterReader = nil
				continue
			}
			return nil, err
		}

		if packet != nil {
			return packet, nil
		}
	}
}

func (pr *PacketReader) loadNextCluster() error {
	if len(pr.parser.clusters) == 0 {
		return pr.scanForClusters()
	}

	if pr.currentCluster == nil {
		pr.currentCluster = pr.parser.clusters[0]
	} else {
		found := false
		for i, cluster := range pr.parser.clusters {
			if cluster.Position == pr.currentCluster.Position && i+1 < len(pr.parser.clusters) {
				pr.currentCluster = pr.parser.clusters[i+1]
				found = true
				break
			}
		}
		if !found {
			return io.EOF
		}
	}

	return pr.loadClusterData()
}

func (pr *PacketReader) scanForClusters() error {
	if err := pr.parser.reader.Seek(pr.parser.segmentPos); err != nil {
		return err
	}

	endPos := pr.parser.segmentPos + pr.parser.segmentSize

	for pr.parser.reader.Position() < endPos {
		element, err := pr.parser.reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if element.ID == ClusterID {
			cluster, errParseClusterInfo := pr.parser.parseClusterInfo(element)
			if errParseClusterInfo != nil {
				return errParseClusterInfo
			}
			pr.parser.clusters = append(pr.parser.clusters, cluster)

			if pr.currentCluster == nil {
				pr.currentCluster = cluster
				return pr.loadClusterData()
			}
		} else {
			if pr.parser.reader.Position() < endPos {
				_ = pr.parser.reader.Seek(element.Offset + getElementHeaderSize(element) + element.Size)
			}
		}
	}

	if pr.currentCluster == nil {
		return io.EOF
	}

	return nil
}

func (pr *PacketReader) loadClusterData() error {
	if err := pr.parser.reader.Seek(pr.currentCluster.Position); err != nil {
		return err
	}

	element, err := pr.parser.reader.ReadElement()
	if err != nil {
		return err
	}

	if element.ID != ClusterID {
		return errors.New("expected cluster element")
	}

	// For Cluster elements, we need to manually read the data since ReadElement doesn't read it
	if element.Data == nil {
		// Seek to the start of cluster data (after header)
		dataStart := element.Offset + element.HeaderSize
		if err = pr.parser.reader.Seek(dataStart); err != nil {
			return err
		}

		// Read cluster data
		clusterData := make([]byte, element.Size)
		n, errReadFull := io.ReadFull(pr.parser.reader.reader, clusterData)
		if errReadFull != nil {
			return errReadFull
		}
		pr.clusterData = clusterData[:n]
	} else {
		pr.clusterData = element.Data
	}

	pr.clusterReader = NewEBMLReader(&bytesReader{data: pr.clusterData})

	return nil
}

func (pr *PacketReader) readPacketFromCluster(mask uint64) (*Packet, error) {
	for pr.clusterReader.Position() < uint64(len(pr.clusterData)) {
		element, err := pr.clusterReader.ReadElement()
		if err != nil {
			if err == io.EOF {
				return nil, io.EOF
			}
			return nil, err
		}

		switch element.ID {
		case TimecodeID:
			timecode, errReadUint := element.ReadUint()
			if errReadUint != nil {
				return nil, errReadUint
			}
			pr.currentCluster.Timecode = timecode
		case SimpleBlockID:
			return pr.parseSimpleBlock(element, mask)
		case BlockGroupID:
			return pr.parseBlockGroup(element, mask)
		}
	}

	return nil, io.EOF
}

func (pr *PacketReader) parseSimpleBlock(element *EBMLElement, mask uint64) (*Packet, error) {
	if len(element.Data) < 4 {
		return nil, errors.New("invalid simple block")
	}

	data := element.Data
	trackNum, trackSize := pr.readVINT(data)

	if trackSize >= len(data) {
		return nil, errors.New("invalid track number in simple block")
	}

	if (mask & (1 << trackNum)) != 0 {
		return nil, nil
	}

	data = data[trackSize:]
	if len(data) < 3 {
		return nil, errors.New("invalid simple block timestamp")
	}

	timestamp := int16(binary.BigEndian.Uint16(data[0:2]))
	flags := data[2]
	data = data[3:]

	packet := &Packet{
		Track:     uint8(trackNum),
		StartTime: pr.currentCluster.Timecode + uint64(timestamp),
		EndTime:   pr.currentCluster.Timecode + uint64(timestamp),
		FilePos:   element.Offset,
		Flags:     uint32(flags),
		Data:      data,
	}

	if flags&0x80 != 0 {
		packet.Flags |= KF
	}

	return packet, nil
}

func (pr *PacketReader) parseBlockGroup(element *EBMLElement, mask uint64) (*Packet, error) {
	reader := NewEBMLReader(&bytesReader{data: element.Data})

	var blockData []byte
	var duration uint64
	var referenceBlocks []int64

	for reader.Position() < uint64(len(element.Data)) {
		child, err := reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		switch child.ID {
		case BlockID:
			blockData = child.Data
		case BlockDurationID:
			dur, errReadUint := child.ReadUint()
			if errReadUint != nil {
				return nil, errReadUint
			}
			duration = dur
		case ReferenceBlockID:
			ref, errReadInt := child.ReadInt()
			if errReadInt != nil {
				return nil, errReadInt
			}
			referenceBlocks = append(referenceBlocks, ref)
		}
	}

	if len(blockData) == 0 {
		return nil, errors.New("no block data in block group")
	}

	packet, err := pr.parseBlockData(blockData, mask, element.Offset)
	if err != nil {
		return nil, err
	}

	if packet == nil {
		return nil, nil
	}

	if duration > 0 {
		packet.EndTime = packet.StartTime + duration
	}

	if len(referenceBlocks) == 0 {
		packet.Flags |= KF
	}

	return packet, nil
}

func (pr *PacketReader) parseBlockData(data []byte, mask uint64, offset uint64) (*Packet, error) {
	if len(data) < 4 {
		return nil, errors.New("invalid block data")
	}

	trackNum, trackSize := pr.readVINT(data)

	if (mask & (1 << trackNum)) != 0 {
		return nil, nil
	}

	data = data[trackSize:]
	if len(data) < 3 {
		return nil, errors.New("invalid block timestamp")
	}

	timestamp := int16(binary.BigEndian.Uint16(data[0:2]))
	flags := data[2]
	data = data[3:]

	packet := &Packet{
		Track:     uint8(trackNum),
		StartTime: pr.currentCluster.Timecode + uint64(timestamp),
		EndTime:   pr.currentCluster.Timecode + uint64(timestamp),
		FilePos:   offset,
		Flags:     uint32(flags),
		Data:      data,
	}

	return packet, nil
}

func (pr *PacketReader) readVINT(data []byte) (uint64, int) {
	if len(data) == 0 {
		return 0, 0
	}

	first := data[0]
	if first == 0 {
		return 0, 0
	}

	var width int
	var mask byte
	for i := 7; i >= 0; i-- {
		if (first & (1 << i)) != 0 {
			width = 8 - i
			mask = byte((1 << i) - 1)
			break
		}
	}

	if width == 0 || width > len(data) {
		return 0, 0
	}

	value := uint64(first & mask)
	for i := 1; i < width; i++ {
		value = (value << 8) | uint64(data[i])
	}

	return value, width
}

func (pr *PacketReader) Seek(timecode uint64, flags uint32) error {
	pr.currentCluster = nil
	pr.clusterReader = nil

	if len(pr.parser.cues) > 0 {
		return pr.seekWithCues(timecode, flags)
	}

	return pr.seekLinear(timecode, flags)
}

func (pr *PacketReader) seekWithCues(timecode uint64, flags uint32) error {
	var bestCue *Cue

	for _, cue := range pr.parser.cues {
		if cue.Time <= timecode {
			if bestCue == nil || cue.Time > bestCue.Time {
				bestCue = cue
			}
		}
	}

	if bestCue == nil {
		bestCue = pr.parser.cues[0]
	}

	clusterPos := pr.parser.segmentPos + bestCue.Position

	for _, cluster := range pr.parser.clusters {
		if cluster.Position == clusterPos {
			pr.currentCluster = cluster
			return pr.loadClusterData()
		}
	}

	if err := pr.parser.reader.Seek(clusterPos); err != nil {
		return err
	}

	element, err := pr.parser.reader.ReadElement()
	if err != nil {
		return err
	}

	if element.ID != ClusterID {
		return errors.New("expected cluster at cue position")
	}

	cluster, err := pr.parser.parseClusterInfo(element)
	if err != nil {
		return err
	}

	pr.currentCluster = cluster
	return pr.loadClusterData()
}

func (pr *PacketReader) seekLinear(timecode uint64, flags uint32) error {
	if err := pr.parser.reader.Seek(pr.parser.segmentPos); err != nil {
		return err
	}

	endPos := pr.parser.segmentPos + pr.parser.segmentSize

	for pr.parser.reader.Position() < endPos {
		element, err := pr.parser.reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if element.ID == ClusterID {
			cluster, errParseClusterInfo := pr.parser.parseClusterInfo(element)
			if errParseClusterInfo != nil {
				return errParseClusterInfo
			}

			if cluster.Timecode >= timecode {
				pr.currentCluster = cluster
				return pr.loadClusterData()
			}
		}

		if pr.parser.reader.Position() < endPos {
			_ = pr.parser.reader.Seek(element.Offset + getElementHeaderSize(element) + element.Size)
		}
	}

	return errors.New("timecode not found")
}

func (pr *PacketReader) GetLowestQTimecode() uint64 {
	return pr.lowestTimecode
}

func (pr *PacketReader) SkipToKeyframe() error {
	for {
		packet, err := pr.ReadPacket()
		if err != nil {
			return err
		}

		if packet.Flags&KF != 0 {
			return nil
		}
	}
}
