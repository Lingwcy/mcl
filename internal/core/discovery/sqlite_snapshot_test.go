package discovery

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func snapshotFixture() []byte {
	data := make([]byte, 512)
	copy(data, "SQLite format 3\x00")
	binary.BigEndian.PutUint16(data[16:18], 512)
	data[18], data[19] = 2, 2
	binary.BigEndian.PutUint32(data[28:32], 1)
	binary.BigEndian.PutUint32(data[56:60], 1)
	return data
}
func walFixture(order binary.ByteOrder) []byte {
	header := make([]byte, 32)
	magic := uint32(0x377f0682)
	if order == binary.BigEndian {
		magic++
	}
	binary.BigEndian.PutUint32(header[:4], magic)
	binary.BigEndian.PutUint32(header[4:8], 3007000)
	binary.BigEndian.PutUint32(header[8:12], 512)
	copy(header[16:24], []byte("salt1234"))
	a, b := walChecksum(order, header[:24], 0, 0)
	binary.BigEndian.PutUint32(header[24:28], a)
	binary.BigEndian.PutUint32(header[28:32], b)
	result := header
	for index := 0; index < 2; index++ {
		frame := make([]byte, 24+512)
		binary.BigEndian.PutUint32(frame[:4], 2)
		if index == 0 {
			binary.BigEndian.PutUint32(frame[4:8], 2)
		}
		copy(frame[8:16], header[16:24])
		copy(frame[24:], []byte{"c"[0] + byte(index)})
		a, b = walChecksum(order, frame[:8], a, b)
		a, b = walChecksum(order, frame[24:], a, b)
		binary.BigEndian.PutUint32(frame[16:20], a)
		binary.BigEndian.PutUint32(frame[20:24], b)
		result = append(result, frame...)
	}
	return result
}
func TestSQLiteSnapshotCommitsOnlyVerifiedFramesWithoutSourceWrites(t *testing.T) {
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		db, wal := snapshotFixture(), walFixture(order)
		originalDB, originalWAL := bytes.Clone(db), bytes.Clone(wal)
		snapshot, err := SQLiteSnapshot(db, wal)
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshot) != 1024 || snapshot[512] != 'c' || snapshot[18] != 1 || snapshot[19] != 1 {
			t.Fatal("uncommitted frame exposed or snapshot mode incorrect")
		}
		if !bytes.Equal(db, originalDB) || !bytes.Equal(wal, originalWAL) {
			t.Fatal("foreign bytes changed")
		}
	}
}
func TestSQLiteSnapshotRejectsCorruptChecksumsAndPageHeaders(t *testing.T) {
	db, wal := snapshotFixture(), walFixture(binary.LittleEndian)
	wal[32+24+40] ^= 1
	if _, err := SQLiteSnapshot(db, wal); err == nil {
		t.Fatal("corrupt WAL accepted")
	}
	wal = walFixture(binary.LittleEndian)
	wal[16] ^= 1
	if _, err := SQLiteSnapshot(db, wal); err == nil {
		t.Fatal("corrupt WAL header accepted")
	}
	db[16], db[17] = 0, 3
	if _, err := SQLiteSnapshot(db, nil); err == nil {
		t.Fatal("invalid page size accepted")
	}
}
