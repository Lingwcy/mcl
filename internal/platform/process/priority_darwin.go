package process

import "golang.org/x/sys/unix"

func childNice(pid int) (int, error) { return unix.Getpriority(unix.PRIO_PROCESS, pid) }

func inheritGatePriority() error { return nil }
