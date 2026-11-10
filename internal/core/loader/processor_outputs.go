package loader

import (
	"context"
	"errors"
	"maps"
	"os"
	"slices"

	"mcl/internal/core/meta"
)

// Files are still disposable staging. Publish their identities to later steps
// only when the ENTIRE output set and its aggregate budget have been checked.
func commitProcessorOutputs(ctx context.Context, stage *os.Root, known map[string]meta.Download, p preparedProcessor) error {
	pending := make(map[string]meta.Download, len(p.outputs))
	for _, target := range slices.Sorted(maps.Keys(p.outputs)) {
		if err := ctx.Err(); err != nil {
			return err
		}
		identity, err := hashForgeFile(ctx, stage, target, p.outputs[target], 0)
		if err != nil {
			return err
		}
		pending[target] = identity
	}
	var bytes int64
	for name, identity := range known {
		if _, replaced := pending[name]; !replaced {
			bytes += identity.Size
		}
	}
	files := len(known)
	for name, identity := range pending {
		bytes += identity.Size
		if _, exists := known[name]; !exists {
			files++
		}
	}
	if bytes > 4<<30 {
		return errors.New("处理器输入及生成文件累计超过4GiB")
	}
	if files > 8192 {
		return errors.New("处理器文件身份数量超过边界")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	maps.Copy(known, pending)
	return nil
}
