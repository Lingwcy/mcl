package platform

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"

	"mcl/internal/app"
)

func NewMemoryProbe() app.MemoryProbe {
	return func(ctx context.Context) (app.MemoryStatus, error) {
		if err := ctx.Err(); err != nil {
			return app.MemoryStatus{}, err
		}
		file, err := os.Open("/proc/meminfo")
		if err != nil {
			return app.MemoryStatus{}, errors.New("无法读取系统内存统计")
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
		if err != nil || len(data) > 64<<10 {
			return app.MemoryStatus{}, errors.New("系统内存统计读取失败或超出上限")
		}
		if err := ctx.Err(); err != nil {
			return app.MemoryStatus{}, err
		}
		return parseLinuxMemoryStatus(string(data))
	}
}
func parseLinuxMemoryStatus(text string) (app.MemoryStatus, error) {
	var status app.MemoryStatus
	seen := map[string]bool{}
	for line := range strings.SplitSeq(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || (fields[0] != "MemTotal:" && fields[0] != "MemAvailable:") {
			continue
		}
		if len(fields) != 3 || fields[2] != "kB" || seen[fields[0]] {
			return status, errors.New("系统内存统计格式无效")
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil || value > ^uint64(0)/1024 {
			return status, errors.New("系统内存统计数值无效")
		}
		seen[fields[0]] = true
		if fields[0] == "MemTotal:" {
			status.TotalBytes = value * 1024
		} else {
			status.AvailableBytes = value * 1024
		}
	}
	if !seen["MemTotal:"] || !seen["MemAvailable:"] {
		return status, errors.New("当前系统未提供可用内存统计")
	}
	return status, status.Validate()
}
