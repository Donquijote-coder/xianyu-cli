package core

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
)

// MessagePackDecoder decodes MessagePack binary data.
type MessagePackDecoder struct {
	data []byte
	pos  int
}

// NewMessagePackDecoder creates a decoder from raw bytes.
func NewMessagePackDecoder(data []byte) *MessagePackDecoder {
	return &MessagePackDecoder{data: data}
}

func (d *MessagePackDecoder) readByte() (byte, error) {
	if d.pos >= len(d.data) {
		return 0, fmt.Errorf("unexpected end of data")
	}
	b := d.data[d.pos]
	d.pos++
	return b, nil
}

func (d *MessagePackDecoder) readBytes(n int) ([]byte, error) {
	if d.pos+n > len(d.data) {
		return nil, fmt.Errorf("unexpected end of data")
	}
	result := d.data[d.pos : d.pos+n]
	d.pos += n
	return result, nil
}

func (d *MessagePackDecoder) readUint16() (uint16, error) {
	b, err := d.readBytes(2)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(b), nil
}

func (d *MessagePackDecoder) readUint32() (uint32, error) {
	b, err := d.readBytes(4)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(b), nil
}

func (d *MessagePackDecoder) readString(length int) (string, error) {
	b, err := d.readBytes(length)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// DecodeValue decodes the next MessagePack value.
func (d *MessagePackDecoder) DecodeValue() (interface{}, error) {
	b, err := d.readByte()
	if err != nil {
		return nil, err
	}

	// Positive fixint (0x00 - 0x7f)
	if b <= 0x7F {
		return int(b), nil
	}
	// Fixmap (0x80 - 0x8f)
	if b >= 0x80 && b <= 0x8F {
		return d.decodeMap(int(b & 0x0F))
	}
	// Fixarray (0x90 - 0x9f)
	if b >= 0x90 && b <= 0x9F {
		return d.decodeArray(int(b & 0x0F))
	}
	// Fixstr (0xa0 - 0xbf)
	if b >= 0xA0 && b <= 0xBF {
		return d.readString(int(b & 0x1F))
	}
	// Nil
	if b == 0xC0 {
		return nil, nil
	}
	// False
	if b == 0xC2 {
		return false, nil
	}
	// True
	if b == 0xC3 {
		return true, nil
	}
	// bin 8
	if b == 0xC4 {
		length, err := d.readByte()
		if err != nil {
			return nil, err
		}
		return d.readBytes(int(length))
	}
	// bin 16
	if b == 0xC5 {
		length, err := d.readUint16()
		if err != nil {
			return nil, err
		}
		return d.readBytes(int(length))
	}
	// bin 32
	if b == 0xC6 {
		length, err := d.readUint32()
		if err != nil {
			return nil, err
		}
		return d.readBytes(int(length))
	}
	// float 32
	if b == 0xCA {
		bs, err := d.readBytes(4)
		if err != nil {
			return nil, err
		}
		bits := binary.BigEndian.Uint32(bs)
		return math.Float32frombits(bits), nil
	}
	// float 64
	if b == 0xCB {
		bs, err := d.readBytes(8)
		if err != nil {
			return nil, err
		}
		bits := binary.BigEndian.Uint64(bs)
		return math.Float64frombits(bits), nil
	}
	// uint 8
	if b == 0xCC {
		v, err := d.readByte()
		return int(v), err
	}
	// uint 16
	if b == 0xCD {
		v, err := d.readUint16()
		return int(v), err
	}
	// uint 32
	if b == 0xCE {
		v, err := d.readUint32()
		return int(v), err
	}
	// uint 64
	if b == 0xCF {
		bs, err := d.readBytes(8)
		if err != nil {
			return nil, err
		}
		return int64(binary.BigEndian.Uint64(bs)), nil
	}
	// int 8
	if b == 0xD0 {
		v, err := d.readByte()
		return int(int8(v)), err
	}
	// int 16
	if b == 0xD1 {
		v, err := d.readUint16()
		return int(int16(v)), err
	}
	// int 32
	if b == 0xD2 {
		v, err := d.readUint32()
		return int(int32(v)), err
	}
	// int 64
	if b == 0xD3 {
		bs, err := d.readBytes(8)
		if err != nil {
			return nil, err
		}
		return int64(binary.BigEndian.Uint64(bs)), nil
	}
	// fixext 1
	if b == 0xD4 {
		_, err := d.readBytes(2) // type + 1 byte data
		return nil, err
	}
	// fixext 2
	if b == 0xD5 {
		_, err := d.readBytes(3)
		return nil, err
	}
	// fixext 4
	if b == 0xD6 {
		_, err := d.readBytes(5)
		return nil, err
	}
	// fixext 8
	if b == 0xD7 {
		_, err := d.readBytes(9)
		return nil, err
	}
	// fixext 16
	if b == 0xD8 {
		_, err := d.readBytes(17)
		return nil, err
	}
	// str 8
	if b == 0xD9 {
		length, err := d.readByte()
		if err != nil {
			return nil, err
		}
		return d.readString(int(length))
	}
	// str 16
	if b == 0xDA {
		length, err := d.readUint16()
		if err != nil {
			return nil, err
		}
		return d.readString(int(length))
	}
	// str 32
	if b == 0xDB {
		length, err := d.readUint32()
		if err != nil {
			return nil, err
		}
		return d.readString(int(length))
	}
	// array 16
	if b == 0xDC {
		n, err := d.readUint16()
		if err != nil {
			return nil, err
		}
		return d.decodeArray(int(n))
	}
	// array 32
	if b == 0xDD {
		n, err := d.readUint32()
		if err != nil {
			return nil, err
		}
		return d.decodeArray(int(n))
	}
	// map 16
	if b == 0xDE {
		n, err := d.readUint16()
		if err != nil {
			return nil, err
		}
		return d.decodeMap(int(n))
	}
	// map 32
	if b == 0xDF {
		n, err := d.readUint32()
		if err != nil {
			return nil, err
		}
		return d.decodeMap(int(n))
	}
	// Negative fixint (0xe0 - 0xff)
	if b >= 0xE0 {
		return int(b) - 256, nil
	}

	return nil, nil
}

func (d *MessagePackDecoder) decodeMap(size int) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	for i := 0; i < size; i++ {
		key, err := d.DecodeValue()
		if err != nil {
			return nil, err
		}
		value, err := d.DecodeValue()
		if err != nil {
			return nil, err
		}
		result[fmt.Sprint(key)] = value
	}
	return result, nil
}

func (d *MessagePackDecoder) decodeArray(size int) ([]interface{}, error) {
	result := make([]interface{}, size)
	for i := 0; i < size; i++ {
		v, err := d.DecodeValue()
		if err != nil {
			return nil, err
		}
		result[i] = v
	}
	return result, nil
}

// DecryptMessage decodes a WebSocket push message.
// Flow: Base64 decode → MessagePack decode → parsed object
func DecryptMessage(data string) interface{} {
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		// Try direct JSON
		var result interface{}
		if json.Unmarshal([]byte(data), &result) == nil {
			return result
		}
		return data
	}

	decoder := NewMessagePackDecoder(raw)
	result, err := decoder.DecodeValue()
	if err != nil {
		// Try direct JSON
		var jsonResult interface{}
		if json.Unmarshal([]byte(data), &jsonResult) == nil {
			return jsonResult
		}
		return data
	}
	return result
}
