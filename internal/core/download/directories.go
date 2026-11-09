package download

import (
	"errors"
	"os"
	"strings"

	"mcl/internal/core/meta"
)

func targetDirectory(root *os.Root, name string, create bool) (*os.Root, error) {
	if name != "." && !meta.ValidPath(name) {
		return nil, errors.New("下载目录路径无效")
	}
	current, err := root.OpenRoot(".")
	if err != nil {
		return nil, err
	}
	if name == "." {
		return current, nil
	}
	for part := range strings.SplitSeq(name, "/") {
		if create {
			if err := current.Mkdir(part, 0700); err != nil && !errors.Is(err, os.ErrExist) {
				current.Close()
				return nil, err
			}
		}
		next, err := realChild(current, part)
		current.Close()
		if err != nil {
			return nil, err
		}
		current = next
	}
	return current, nil
}
