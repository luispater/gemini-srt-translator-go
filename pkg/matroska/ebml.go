package matroska

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
)

var (
	ErrInvalidEBML     = errors.New("invalid EBML data")
	ErrElementTooLarge = errors.New("element size too large")
)

type EBMLElement struct {
	ID         uint32
	Size       uint64
	Data       []byte
	Offset     uint64
	HeaderSize uint64
	Children   []*EBMLElement
}

type EBMLReader struct {
	reader io.ReadSeeker
	pos    uint64
}

func NewEBMLReader(r io.ReadSeeker) *EBMLReader {
	return &EBMLReader{reader: r, pos: 0}
}

func (r *EBMLReader) ReadVINT() (uint64, int, error) {
	var b [1]byte
	if _, err := r.reader.Read(b[:]); err != nil {
		return 0, 0, err
	}
	r.pos++

	first := b[0]
	if first == 0 {
		return 0, 0, ErrInvalidEBML
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

	if width == 0 {
		return 0, 0, ErrInvalidEBML
	}

	value := uint64(first & mask)
	for i := 1; i < width; i++ {
		if _, err := r.reader.Read(b[:]); err != nil {
			return 0, 0, err
		}
		r.pos++
		value = (value << 8) | uint64(b[0])
	}

	return value, width, nil
}

func (r *EBMLReader) ReadVINTRaw() (uint64, int, error) {
	var b [1]byte
	if _, err := r.reader.Read(b[:]); err != nil {
		return 0, 0, err
	}
	r.pos++

	first := b[0]
	if first == 0 {
		return 0, 0, ErrInvalidEBML
	}

	var width int
	for i := 7; i >= 0; i-- {
		if (first & (1 << i)) != 0 {
			width = 8 - i
			break
		}
	}

	if width == 0 {
		return 0, 0, ErrInvalidEBML
	}

	// For Element IDs, we keep the length marker bit
	value := uint64(first)
	for i := 1; i < width; i++ {
		if _, err := r.reader.Read(b[:]); err != nil {
			return 0, 0, err
		}
		r.pos++
		value = (value << 8) | uint64(b[0])
	}

	return value, width, nil
}

func (r *EBMLReader) ReadElementID() (uint32, error) {
	id, _, err := r.ReadVINTRaw()
	if err != nil {
		return 0, err
	}
	return uint32(id), nil
}

func (r *EBMLReader) ReadElementSize() (uint64, error) {
	size, _, err := r.ReadVINT()
	if err != nil {
		return 0, err
	}

	if size == (1<<56)-1 {
		return math.MaxUint64, nil
	}

	return size, nil
}

func (r *EBMLReader) ReadElement() (*EBMLElement, error) {
	startPos := r.pos

	id, err := r.ReadElementID()
	if err != nil {
		return nil, err
	}

	size, err := r.ReadElementSize()
	if err != nil {
		return nil, err
	}

	headerSize := r.pos - startPos

	element := &EBMLElement{
		ID:         id,
		Size:       size,
		Offset:     startPos,
		HeaderSize: headerSize,
	}

	if size != math.MaxUint64 {
		// For large container elements, don't read the data into memory
		if element.IsContainer() && (element.ID == 0x18538067 || element.ID == 0x1F43B675) { // Segment or Cluster
			// Just skip reading the data for large container elements
			element.Data = nil
		} else {
			if size > math.MaxInt32 {
				return nil, ErrElementTooLarge
			}

			data := make([]byte, size)
			n, errReadFull := io.ReadFull(r.reader, data)
			if errReadFull != nil {
				return nil, errReadFull
			}
			r.pos += uint64(n)
			element.Data = data
		}
	}

	return element, nil
}

func (r *EBMLReader) Seek(pos uint64) error {
	_, err := r.reader.Seek(int64(pos), io.SeekStart)
	if err != nil {
		return err
	}
	r.pos = pos
	return nil
}

func (r *EBMLReader) Position() uint64 {
	return r.pos
}

func (e *EBMLElement) ReadUint() (uint64, error) {
	if len(e.Data) > 8 {
		return 0, ErrInvalidEBML
	}

	var value uint64
	for _, b := range e.Data {
		value = (value << 8) | uint64(b)
	}
	return value, nil
}

func (e *EBMLElement) ReadInt() (int64, error) {
	if len(e.Data) > 8 {
		return 0, ErrInvalidEBML
	}

	if len(e.Data) == 0 {
		return 0, nil
	}

	var value int64
	if e.Data[0]&0x80 != 0 {
		value = -1
	}

	for _, b := range e.Data {
		value = (value << 8) | int64(b)
	}
	return value, nil
}

func (e *EBMLElement) ReadFloat() (float64, error) {
	switch len(e.Data) {
	case 4:
		bits := binary.BigEndian.Uint32(e.Data)
		return float64(math.Float32frombits(bits)), nil
	case 8:
		bits := binary.BigEndian.Uint64(e.Data)
		return math.Float64frombits(bits), nil
	default:
		return 0, ErrInvalidEBML
	}
}

func (e *EBMLElement) ReadString() string {
	return string(e.Data)
}

func (e *EBMLElement) ReadBytes() []byte {
	result := make([]byte, len(e.Data))
	copy(result, e.Data)
	return result
}

func (e *EBMLElement) IsContainer() bool {
	switch e.ID {
	case 0x1A45DFA3, 0x18538067, 0x1549A966, 0x1654AE6B, 0x1F43B675:
		return true
	case 0xAE, 0x1043A770, 0x1254C367, 0x1941A469:
		return true
	default:
		return false
	}
}

func ParseEBMLChildren(reader *EBMLReader, size uint64) ([]*EBMLElement, error) {
	var children []*EBMLElement
	startPos := reader.Position()
	endPos := startPos + size

	for reader.Position() < endPos {
		element, err := reader.ReadElement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		children = append(children, element)
	}

	return children, nil
}
