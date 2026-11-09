package storage

import (
	"context"
	"errors"
	"io"
	"os"

	"mcl/internal/core/discovery"
)

func readSwiftDocument(ctx context.Context, root *os.Root, name string, consumed *int64) ([]byte, error) {
	before, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() <= 0 || before.Size() > discovery.MaxSQLiteBytes || *consumed+before.Size() > discovery.MaxMetadataBytes {
		return nil, errors.New("Swift实例索引超过读取边界")
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	actual, err := file.Stat()
	if err != nil || !os.SameFile(before, actual) {
		return nil, errors.New("Swift实例索引读取时被替换")
	}
	data, err := io.ReadAll(&swiftReader{ctx: ctx, reader: io.LimitReader(file, before.Size()+1), consumed: consumed})
	if err != nil {
		return nil, err
	}
	after, err := file.Stat()
	if err != nil {
		return nil, err
	}
	current, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !current.Mode().IsRegular() || !os.SameFile(before, after) || !os.SameFile(before, current) || int64(len(data)) != before.Size() || current.Size() != before.Size() || after.Size() != before.Size() || !current.ModTime().Equal(before.ModTime()) || !after.ModTime().Equal(before.ModTime()) {
		return nil, errors.New("Swift实例索引读取期间发生变化，请重新扫描")
	}
	return data, ctx.Err()
}

type swiftReader struct {
	ctx      context.Context
	reader   io.Reader
	consumed *int64
}

func (r *swiftReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.reader.Read(p)
	*r.consumed += int64(n)
	return n, err
}
