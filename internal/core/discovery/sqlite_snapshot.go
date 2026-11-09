package discovery

import (
	"bytes"
	"encoding/binary"
	"errors"
)

const MaxSQLiteBytes = 32 << 20

// SQLiteSnapshot reconstructs only the last committed WAL transaction. Neither
// SQLite nor any foreign database path is opened here. The returned image is
// used in a private, read-only in-memory connection, never written back.
func SQLiteSnapshot(database, wal []byte) ([]byte, error) {
	fail := func() ([]byte, error) {
		return nil, errors.New("实例索引快照无效或超过32MiB，请关闭来源启动器后重试")
	}
	if len(database) < 100 || len(database) > MaxSQLiteBytes || !bytes.Equal(database[:16], []byte("SQLite format 3\x00")) {
		return fail()
	}
	pageSize := int(binary.BigEndian.Uint16(database[16:18]))
	if pageSize == 1 {
		pageSize = 65536
	}
	if pageSize < 512 || pageSize > 65536 || pageSize&(pageSize-1) != 0 || len(database)%pageSize != 0 {
		return fail()
	}
	result := append([]byte(nil), database...)
	if len(wal) > 0 {
		if len(wal) < 32 || len(wal) > MaxSQLiteBytes {
			return fail()
		}
		magic := binary.BigEndian.Uint32(wal[:4])
		if magic != 0x377f0682 && magic != 0x377f0683 || binary.BigEndian.Uint32(wal[4:8]) != 3007000 || int(binary.BigEndian.Uint32(wal[8:12])) != pageSize {
			return fail()
		}
		var order binary.ByteOrder = binary.LittleEndian
		if magic == 0x377f0683 {
			order = binary.BigEndian
		}
		a, b := walChecksum(order, wal[:24], 0, 0)
		if a != binary.BigEndian.Uint32(wal[24:28]) || b != binary.BigEndian.Uint32(wal[28:32]) {
			return fail()
		}
		type frame struct {
			page int
			data []byte
		}
		var frames []frame
		committed, pages := 0, 0
		for offset := 32; offset+24+pageSize <= len(wal); offset += 24 + pageSize {
			header, data := wal[offset:offset+24], wal[offset+24:offset+24+pageSize]
			// Old reusable WAL tail can belong to an earlier salt. SQLite ignores it.
			if !bytes.Equal(header[8:16], wal[16:24]) {
				break
			}
			a, b = walChecksum(order, header[:8], a, b)
			a, b = walChecksum(order, data, a, b)
			if a != binary.BigEndian.Uint32(header[16:20]) || b != binary.BigEndian.Uint32(header[20:24]) {
				return fail()
			}
			page, size := int(binary.BigEndian.Uint32(header[:4])), int(binary.BigEndian.Uint32(header[4:8]))
			if page < 1 || page > MaxSQLiteBytes/pageSize || size > MaxSQLiteBytes/pageSize {
				return fail()
			}
			frames = append(frames, frame{page, data})
			if size != 0 {
				committed, pages = len(frames), size
			}
		}
		if committed > 0 {
			size := pages * pageSize
			if size > len(result) {
				result = append(result, make([]byte, size-len(result))...)
			} else {
				result = result[:size]
			}
			for _, frame := range frames[:committed] {
				if frame.page <= pages {
					copy(result[(frame.page-1)*pageSize:frame.page*pageSize], frame.data)
				}
			}
		}
	}
	// In-memory SQLite must not try to locate an external WAL. These are private
	// snapshot bytes, not mutations to the foreign header or a checkpoint.
	result[18], result[19] = 1, 1
	if len(result) < pageSize || !bytes.Equal(result[:16], []byte("SQLite format 3\x00")) || binary.BigEndian.Uint32(result[56:60]) != 1 {
		return fail()
	}
	return result, nil
}
func walChecksum(order binary.ByteOrder, data []byte, a, b uint32) (uint32, uint32) {
	for offset := 0; offset+8 <= len(data); offset += 8 {
		a += order.Uint32(data[offset:offset+4]) + b
		b += order.Uint32(data[offset+4:offset+8]) + a
	}
	return a, b
}

type SwiftProfile struct{ ID, WorkingPath, Name, GameVersion, Loader, LoaderVersion, MainClass string }
