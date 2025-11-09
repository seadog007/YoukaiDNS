package dns

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

// ParseMessage parses a DNS message from bytes
func ParseMessage(data []byte) (*Message, error) {
	if len(data) < 12 {
		return nil, errors.New("message too short")
	}

	msg := &Message{}
	offset := 0

	// Parse header
	msg.Header.ID = binary.BigEndian.Uint16(data[offset:])
	offset += 2
	msg.Header.Flags = binary.BigEndian.Uint16(data[offset:])
	offset += 2
	msg.Header.QdCount = binary.BigEndian.Uint16(data[offset:])
	offset += 2
	msg.Header.AnCount = binary.BigEndian.Uint16(data[offset:])
	offset += 2
	msg.Header.NsCount = binary.BigEndian.Uint16(data[offset:])
	offset += 2
	msg.Header.ArCount = binary.BigEndian.Uint16(data[offset:])
	offset += 2

	// Parse questions
	msg.Questions = make([]Question, msg.Header.QdCount)
	for i := uint16(0); i < msg.Header.QdCount; i++ {
		name, newOffset, err := decodeName(data, offset)
		if err != nil {
			return nil, err
		}
		offset = newOffset

		if offset+4 > len(data) {
			return nil, errors.New("message too short for question")
		}

		msg.Questions[i].Name = name
		msg.Questions[i].Type = binary.BigEndian.Uint16(data[offset:])
		offset += 2
		msg.Questions[i].Class = binary.BigEndian.Uint16(data[offset:])
		offset += 2
	}

	return msg, nil
}

// BuildResponse builds a DNS response message
func BuildResponse(query *Message, answers []ResourceRecord, rcode int) (*Message, error) {
	response := &Message{
		Header: MessageHeader{
			ID:      query.Header.ID,
			Flags:   FlagResponse | uint16(rcode),
			QdCount: query.Header.QdCount,
			AnCount: uint16(len(answers)),
			NsCount: 0,
			ArCount: 0,
		},
		Questions: query.Questions,
		Answers:   answers,
	}

	return response, nil
}

// ToBytes converts a DNS message to bytes
func (m *Message) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Write header
	binary.Write(buf, binary.BigEndian, m.Header.ID)
	binary.Write(buf, binary.BigEndian, m.Header.Flags)
	binary.Write(buf, binary.BigEndian, m.Header.QdCount)
	binary.Write(buf, binary.BigEndian, m.Header.AnCount)
	binary.Write(buf, binary.BigEndian, m.Header.NsCount)
	binary.Write(buf, binary.BigEndian, m.Header.ArCount)

	// Write questions
	for _, q := range m.Questions {
		if err := encodeName(buf, q.Name); err != nil {
			return nil, err
		}
		binary.Write(buf, binary.BigEndian, q.Type)
		binary.Write(buf, binary.BigEndian, q.Class)
	}

	// Write answers
	for _, rr := range m.Answers {
		if err := encodeName(buf, rr.Name); err != nil {
			return nil, err
		}
		binary.Write(buf, binary.BigEndian, rr.Type)
		binary.Write(buf, binary.BigEndian, rr.Class)
		binary.Write(buf, binary.BigEndian, rr.TTL)
		binary.Write(buf, binary.BigEndian, rr.DataLen)
		buf.Write(rr.Data)
	}

	return buf.Bytes(), nil
}

// decodeName decodes a DNS name from bytes
func decodeName(data []byte, offset int) (string, int, error) {
	var name []byte
	originalOffset := offset
	jumped := false
	maxJumps := 5
	jumpsPerformed := 0

	for {
		if offset >= len(data) {
			return "", 0, errors.New("invalid name: offset out of bounds")
		}

		length := int(data[offset])
		offset++

		if length == 0 {
			break
		}

		// Check for compression pointer (two high bits set)
		if length&0xC0 == 0xC0 {
			if !jumped {
				originalOffset = offset + 1
			}
			jumped = true
			jumpsPerformed++
			if jumpsPerformed > maxJumps {
				return "", 0, errors.New("too many compression jumps")
			}

			// Extract pointer offset
			pointer := binary.BigEndian.Uint16(data[offset-1:]) & 0x3FFF
			offset = int(pointer)
			continue
		}

		// Regular label
		if offset+length > len(data) {
			return "", 0, errors.New("invalid name: label out of bounds")
		}

		if len(name) > 0 {
			name = append(name, '.')
		}
		name = append(name, data[offset:offset+length]...)
		offset += length
	}

	if jumped {
		offset = originalOffset
	}

	return string(name), offset, nil
}

// encodeName encodes a DNS name to bytes
func encodeName(buf *bytes.Buffer, name string) error {
	if len(name) == 0 {
		buf.WriteByte(0)
		return nil
	}

	parts := bytes.Split([]byte(name), []byte("."))
	for _, part := range parts {
		if len(part) > 63 {
			return fmt.Errorf("label too long: %s", string(part))
		}
		buf.WriteByte(byte(len(part)))
		buf.Write(part)
	}
	buf.WriteByte(0) // Null terminator

	return nil
}

