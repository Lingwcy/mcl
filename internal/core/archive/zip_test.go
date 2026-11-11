package archive

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

func streamFixture(t *testing.T) ([]byte, []byte) {
	t.Helper()
	payload := bytes.Repeat([]byte("bounded-streaming-payload"), 4096)
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	f, err := w.CreateHeader(&zip.FileHeader{Name: "file.bin", Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes(), payload
}

type failingWriter struct{ cause error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.cause }

func TestArchiveCopyStreamsAndChecksBoundsCRCAndCancellation(t *testing.T) {
	raw, payload := streamFixture(t)
	z, err := Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	n, err := z.Copy(t.Context(), "file.bin", &out, int64(len(payload)))
	if err != nil || n != int64(len(payload)) || !bytes.Equal(out.Bytes(), payload) {
		t.Fatal("streamed bytes differ", n, err)
	}
	out.Reset()
	if _, err := z.Copy(t.Context(), "file.bin", &out, int64(len(payload)-1)); err == nil || out.Len() != 0 {
		t.Fatal("oversized entry partially copied")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := z.Copy(ctx, "file.bin", io.Discard, int64(len(payload))); !errors.Is(err, context.Canceled) {
		t.Fatal("copy lost cancellation", err)
	}
	sentinel := errors.New("destination rejected bytes")
	if _, err := z.Copy(t.Context(), "file.bin", failingWriter{sentinel}, int64(len(payload))); !errors.Is(err, sentinel) {
		t.Fatal("writer failure lost", err)
	}
	// Stored entry bytes can be changed without changing the ZIP inventory.
	damaged := bytes.Clone(raw)
	offset := bytes.Index(damaged, payload)
	if offset < 0 {
		t.Fatal("stored payload not present")
	}
	damaged[offset] ^= 1
	corrupt, err := Open(bytes.NewReader(damaged), int64(len(damaged)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := corrupt.Copy(t.Context(), "file.bin", io.Discard, int64(len(payload))); !errors.Is(err, zip.ErrChecksum) {
		t.Fatal("full stream did not check CRC", err)
	}
}
