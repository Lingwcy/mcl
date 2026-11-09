package fileio

import "os"

// Windows has no unprivileged POSIX directory-fsync equivalent. Callers flush
// copied files and reopened checkpoints after rename and retain the old copy.
// Recovery still depends on the filesystem's metadata-journaling guarantees;
// this function must not be advertised as a multi-file power-loss transaction.
func SyncDirectory(root *os.Root) error { return nil }
