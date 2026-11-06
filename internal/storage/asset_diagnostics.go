package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"path"

	"mcl/internal/app"
	"mcl/internal/core/download"
	"mcl/internal/core/market"
)

func (s *AssetStore) DiagnosticSnapshot(ctx context.Context, id string, report func(download.Progress)) (app.AssetDiagnosticSnapshot, error) {
	result := app.AssetDiagnosticSnapshot{}
	snapshot, err := s.List(ctx, id)
	if err != nil {
		return result, err
	}
	if snapshot.Warning != "" {
		return result, errors.New(snapshot.Warning)
	}
	if snapshot.PendingUpdate != nil {
		return result, errors.New("请先恢复未完成的资产更新")
	}
	inspector := &modInspector{}
	hashes, err := s.hashAssetsWithInspector(ctx, id, market.Modrinth, snapshot, report, func(file app.AssetFile, reader io.ReaderAt, size int64) {
		inspector.inspect(ctx, file, reader, size)
	})
	if err != nil {
		return result, err
	}
	result.Truncated, result.Warnings = snapshot.Truncated || hashes.Truncated, hashes.Warnings
	result.Local = inspector.observations(snapshot.Files, hashes.Files)
	result.Input.Truncated = result.Truncated
	byPath := make(map[string]market.FileDigest, len(hashes.Files))
	for _, file := range hashes.Files {
		byPath[file.File.Path] = file.Digest
	}
	for _, file := range snapshot.Files {
		evidence := market.AssetEvidence{Path: file.Path, Kind: market.Kind(file.Kind), Enabled: file.Enabled, Directory: file.Directory, Size: file.Size}
		if hash, ok := byPath[file.Path]; ok {
			evidence.Checked, evidence.SHA1 = true, hash.SHA1
		}
		result.Input.Files = append(result.Input.Files, evidence)
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return result, err
	}
	defer root.Close()
	receipt, err := readMarketReceiptAt(ctx, root, path.Join(id, "market.json"))
	if err != nil {
		return result, err
	}
	result.Input.Records = receipt.Files
	return result, ctx.Err()
}
