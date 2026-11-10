package process

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"mcl/internal/core/diagnostics"
	"mcl/internal/core/launch"
)

// Retain the existing hard execution bound pending measured installation data.
// Output is evidence only; silence never starts an independent kill timer.
const processorExecutionTimeout = 4 * time.Minute

type processorExecutionError struct {
	kind  string
	cause error
	bytes uint64
	tail  []string
}

func (e *processorExecutionError) Unwrap() error { return e.cause }
func (e *processorExecutionError) DiagnosticCode() string {
	if e.kind == "timeout" {
		return "deadline"
	}
	return diagnostics.Code(e.cause)
}
func (e *processorExecutionError) Error() string {
	label := "安装处理器进程失败"
	switch e.kind {
	case "start":
		label = "无法启动Java安装处理器"
	case "attach":
		label = "无法建立安装处理器进程树控制"
	case "timeout":
		label = "安装处理器执行超时（4分钟上限）"
	case "drain":
		label = "安装处理器输出未在期限内排空"
	case "exit":
		var exit *exec.ExitError
		if errors.As(e.cause, &exit) {
			label = fmt.Sprintf("安装处理器退出，代码%d", exit.ExitCode())
			if exit.ExitCode() < 0 && exit.ProcessState != nil {
				label = "安装处理器退出（" + exit.ProcessState.String() + "）"
			}
		}
	}
	if e.kind == "start" || e.kind == "attach" {
		switch diagnostics.Code(e.cause) {
		case "permission":
			label += "：权限不足"
		case "disk-full":
			label += "：磁盘空间不足"
		case "deadline":
			label += "：操作超时"
		}
	}
	if e.bytes == 0 {
		label += "；未收到标准输出或错误输出"
	} else {
		label += fmt.Sprintf("；已收到%d字节输出", e.bytes)
	}
	if len(e.tail) > 0 {
		label += "\n" + strings.Join(e.tail, "\n")
	}
	return label
}
func processorRunFailure(kind string, cause error, bytes uint64, output *launch.Output) error {
	failure := &processorExecutionError{kind: kind, cause: cause, bytes: bytes}
	lines := output.Snapshot().Lines
	for _, line := range lines[max(0, len(lines)-3):] {
		text := strings.TrimSpace(line.Text)
		runes := []rune(text)
		if len(runes) > 300 {
			text = string(runes[:300])
		}
		if text != "" {
			failure.tail = append(failure.tail, text)
		}
	}
	return failure
}
