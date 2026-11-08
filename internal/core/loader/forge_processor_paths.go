package loader

import (
	"errors"
	"path"

	"mcl/internal/core/archive"
)

func validateProcessorPaths(libraries map[string]forgeLibrary, processors []preparedProcessor) error {
	seen := make(map[string]string)
	add := func(name string) error {
		if !archive.SafePath(name) {
			return errors.New("处理器文件路径无效")
		}
		key := archive.Key(name)
		if previous, exists := seen[key]; exists && previous != name {
			return errors.New("处理器文件路径存在大小写或Unicode别名")
		}
		seen[key] = name
		return nil
	}
	for relative := range libraries {
		if err := add(path.Join("libraries", relative)); err != nil {
			return err
		}
	}
	for _, processor := range processors {
		for name := range processor.outputs {
			if err := add(name); err != nil {
				return err
			}
		}
		for _, name := range processor.inputs {
			if name == "." || name == "libraries" {
				continue
			}
			if err := add(name); err != nil {
				return err
			}
		}
	}
	// A path used as a directory input (MCP data) can have descendants. Its
	// concrete file/directory type is checked against the fixed staging evidence
	// when that step actually runs, not guessed from its filename or extension.
	return nil
}
