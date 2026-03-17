"""MessagePack decoder for XianYu WebSocket messages.

Decodes Base64-encoded MessagePack binary data from syncPushPackage.
This is a minimal pure-Python implementation — no external msgpack dependency.
"""

from __future__ import annotations

import base64
import struct
from typing import Any


class MessagePackDecoder:
    """Pure Python MessagePack decoder for XianYu WebSocket push messages."""

    def __init__(self, data: bytes):
        self.data = data
        self.pos = 0

    def read_byte(self) -> int:
        if self.pos >= len(self.data):
            raise ValueError("Unexpected end of data")
        byte = self.data[self.pos]
        self.pos += 1
        return byte

    def read_bytes(self, n: int) -> bytes:
        if self.pos + n > len(self.data):
            raise ValueError("Unexpected end of data")
        result = self.data[self.pos : self.pos + n]
        self.pos += n
        return result

    def read_uint16(self) -> int:
        return struct.unpack(">H", self.read_bytes(2))[0]

    def read_uint32(self) -> int:
        return struct.unpack(">I", self.read_bytes(4))[0]

    def read_int8(self) -> int:
        return struct.unpack(">b", self.read_bytes(1))[0]

    def read_int16(self) -> int:
        return struct.unpack(">h", self.read_bytes(2))[0]

    def read_int32(self) -> int:
        return struct.unpack(">i", self.read_bytes(4))[0]

    def read_float32(self) -> float:
        return struct.unpack(">f", self.read_bytes(4))[0]

    def read_float64(self) -> float:
        return struct.unpack(">d", self.read_bytes(8))[0]

    def read_string(self, length: int) -> str:
        return self.read_bytes(length).decode("utf-8", errors="replace")

    def decode_value(self) -> Any:
        fmt = self.read_byte()

        # Positive fixint (0x00 - 0x7f)
        if fmt <= 0x7F:
            return fmt
        # Fixmap (0x80 - 0x8f)
        if 0x80 <= fmt <= 0x8F:
            return self.decode_map(fmt & 0x0F)
        # Fixarray (0x90 - 0x9f)
        if 0x90 <= fmt <= 0x9F:
            return self.decode_array(fmt & 0x0F)
        # Fixstr (0xa0 - 0xbf)
        if 0xA0 <= fmt <= 0xBF:
            return self.read_string(fmt & 0x1F)
        # Nil
        if fmt == 0xC0:
            return None
        # False
        if fmt == 0xC2:
            return False
        # True
        if fmt == 0xC3:
            return True
        # bin 8
        if fmt == 0xC4:
            length = self.read_byte()
            return self.read_bytes(length)
        # bin 16
        if fmt == 0xC5:
            length = self.read_uint16()
            return self.read_bytes(length)
        # bin 32
        if fmt == 0xC6:
            length = self.read_uint32()
            return self.read_bytes(length)
        # float 32
        if fmt == 0xCA:
            return self.read_float32()
        # float 64
        if fmt == 0xCB:
            return self.read_float64()
        # uint 8
        if fmt == 0xCC:
            return self.read_byte()
        # uint 16
        if fmt == 0xCD:
            return self.read_uint16()
        # uint 32
        if fmt == 0xCE:
            return self.read_uint32()
        # int 8
        if fmt == 0xD0:
            return self.read_int8()
        # int 16
        if fmt == 0xD1:
            return self.read_int16()
        # int 32
        if fmt == 0xD2:
            return self.read_int32()
        # str 8
        if fmt == 0xD9:
            length = self.read_byte()
            return self.read_string(length)
        # str 16
        if fmt == 0xDA:
            length = self.read_uint16()
            return self.read_string(length)
        # str 32
        if fmt == 0xDB:
            length = self.read_uint32()
            return self.read_string(length)
        # array 16
        if fmt == 0xDC:
            return self.decode_array(self.read_uint16())
        # array 32
        if fmt == 0xDD:
            return self.decode_array(self.read_uint32())
        # map 16
        if fmt == 0xDE:
            return self.decode_map(self.read_uint16())
        # map 32
        if fmt == 0xDF:
            return self.decode_map(self.read_uint32())
        # Negative fixint (0xe0 - 0xff)
        if fmt >= 0xE0:
            return fmt - 256

        return None

    def decode_map(self, size: int) -> dict[str, Any]:
        result: dict[str, Any] = {}
        for _ in range(size):
            key = self.decode_value()
            value = self.decode_value()
            result[str(key) if not isinstance(key, str) else key] = value
        return result

    def decode_array(self, size: int) -> list[Any]:
        return [self.decode_value() for _ in range(size)]


def decrypt_message(data: str) -> Any:
    """Decrypt a WebSocket push message.

    Flow: Base64 decode → MessagePack decode → parsed object
    """
    try:
        raw = base64.b64decode(data)
        decoder = MessagePackDecoder(raw)
        return decoder.decode_value()
    except Exception:
        # Fallback: try direct JSON
        import json
        import logging

        logging.getLogger(__name__).debug("MessagePack decode failed, trying JSON")
        try:
            return json.loads(data)
        except (json.JSONDecodeError, TypeError):
            logging.getLogger(__name__).debug("JSON fallback also failed, returning raw")
            return data
