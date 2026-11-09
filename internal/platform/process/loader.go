package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"mcl/internal/core/java"
	"mcl/internal/core/launch"
	"mcl/internal/core/meta"
)

// RunLoaderProcessor owns a short-lived process tree entirely separate from
// registered game sessions. Its work directory is private installation staging.
func RunLoaderProcessor(ctx context.Context, executable, work string, args []string) error {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	args = append([]string{"-Duser.home=" + work, "-Djava.io.tmpdir=" + work}, args...)
	runtime, err := java.Probe(ctx, executable)
	if err != nil {
		return err
	}
	if runtime.Major > 25 {
		return errors.New("安装工具Java超过已支持范围")
	}
	if err := runtime.Check(8, meta.CurrentPlatform()); err != nil {
		return err
	}
	var data []byte
	extension := ".args"
	if runtime.Major == 8 {
		index := slices.Index(args, "-cp")
		if index < 0 || index+1 >= len(args) {
			return errors.New("Java8安装工具缺少唯一classpath")
		}
		data, err = launch.ClasspathSupportJAR(strings.Split(args[index+1], string(os.PathListSeparator)), meta.CurrentPlatform().OS)
		extension = ".jar"
	} else {
		data, err = java.EncodeArgumentFile(args)
	}
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(work, "processor-*"+extension)
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	commandArgs := []string{"@" + filepath.Base(f.Name())}
	if runtime.Major == 8 {
		commandArgs = slices.Clone(args)
		commandArgs[slices.Index(commandArgs, "-cp")+1] = f.Name()
	}
	if err := validateCommandSize(executable, commandArgs); err != nil {
		return err
	}
	cmd := exec.Command(executable, commandArgs...)
	cmd.Dir = work
	cmd.Env = processorEnvironment()
	cmd.WaitDelay = 2 * time.Second
	configure(cmd)
	output := (launch.Plan{}).NewOutput()
	stdout, stderr := output.Stream("stdout"), output.Stream("stderr")
	defer stdout.Close()
	defer stderr.Close()
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("无法启动 Java 安装处理器: %w", err)
	}
	control, err := attach(cmd, ctx.Err)
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return err
	}
	defer control.close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
		control.kill()
	case <-ctx.Done():
		control.kill()
		<-done
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err == nil {
		return nil
	}
	stdout.Close()
	stderr.Close()
	lines := output.Snapshot().Lines
	var tail []string
	for _, line := range lines[max(0, len(lines)-3):] {
		text := strings.TrimSpace(line.Text)
		runes := []rune(text)
		if len(runes) > 300 {
			text = string(runes[:300])
		}
		if text != "" {
			tail = append(tail, text)
		}
	}
	if len(tail) == 0 {
		return err
	}
	return errors.New(err.Error() + "\n" + strings.Join(tail, "\n"))
}

func processorEnvironment() []string {
	result := make([]string, 0, 8)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "PATH", "SYSTEMROOT", "WINDIR", "LANG", "LC_ALL", "LC_CTYPE", "TMP", "TEMP", "TMPDIR":
			result = append(result, entry)
		}
	}
	return result
}
