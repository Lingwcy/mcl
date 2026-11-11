package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"mcl/internal/core/download"
	"mcl/internal/core/meta"
)

// VerifyLocalLibraries never downloads a generated file or repairs it in place.
// A caller must rebuild its owning loader generation if identity has changed.
func VerifyLocalLibraries(ctx context.Context, root *os.Root, plan Plan) error {
	for _, files := range [][]download.File{plan.LocalLibraries, plan.RuntimeLookups} {
		for _, file := range files {
			if file.URL != "" || file.Download.ValidateIdentity() != nil || !meta.ValidPath(file.Path) || file.Size > 512<<20 {
				return errors.New("受管运行库身份无效")
			}
			handle, identity, err := openAssetFile(root, file.Path, file.Size)
			if err == nil {
				err = streamAsset(ctx, handle, io.Discard, file.Size, file.SHA1)
				if err == nil {
					err = sameAssetFile(root, file.Path, handle, identity)
				}
				handle.Close()
			}
			if err != nil {
				return fmt.Errorf("受管运行库未通过校验，需重新准备加载器: %w", err)
			}
		}
	}
	return ctx.Err()
}
