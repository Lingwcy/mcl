package process

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"mcl/internal/core/diagnostics"
	"mcl/internal/core/java"
	"mcl/internal/core/launch"
	"mcl/internal/core/meta"
	"mcl/internal/platform"
)

// RunLoaderProcessor owns a short-lived process tree entirely separate from
// registered game sessions. Its work directory is private installation staging.
func RunLoaderProcessor(ctx context.Context, executable, work string, args []string) error {
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, processorExecutionTimeout)
	defer cancel()
	args = append([]string{"-Duser.home=" + work, "-Djava.io.tmpdir=" + work}, args...)
	runtime, err := java.Probe(ctx, executable)
	if err != nil {
		return err
	}
	if requirement, ok := java.RequirementFromContext(ctx); ok {
		if requirement.Purpose != "processor" {
			return errors.New("安装工具收到非工具用途的Java需求")
		}
		if err := requirement.CheckRuntime(runtime); err != nil {
			return err
		}
	}
	diagnostics.SetFacts(ctx, runtime.DiagnosticFacts(true))
	if runtime.Major > 25 {
		return errors.New("安装工具Java超过已支持范围")
	}
	host := meta.CurrentPlatform()
	target := host
	target.Arch = runtime.Arch
	if err := platform.CheckExecutionPlatform(ctx, host, target); err != nil {
		return err
	}
	if err := runtime.Check(8, target); err != nil {
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
		data, err = java.EncodeRuntimeArgumentFile(args, host.OS, runtime.NativeEncoding)
	}
	if err != nil {
		return err
	}
	supportRoot, err := os.OpenRoot(work)
	if err != nil {
		return err
	}
	defer supportRoot.Close()
	supportName := "processor-" + rand.Text() + extension
	f, err := supportRoot.OpenFile(supportName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	identity, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	defer removeSupportFile(supportRoot, supportName, identity)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	commandArgs := []string{"@" + supportName}
	if runtime.Major == 8 {
		commandArgs = slices.Clone(args)
		commandArgs[slices.Index(commandArgs, "-cp")+1] = filepath.Join(work, supportName)
	}
	if err := java.ValidateNativeArguments(append([]string{executable}, commandArgs...), host.OS, runtime.NativeEncoding); err != nil {
		return err
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
	var outputBytes atomic.Uint64
	cmd.Stdout, cmd.Stderr = processorEvidenceWriter{ctx: ctx, writer: stdout, bytes: &outputBytes}, processorEvidenceWriter{ctx: ctx, writer: stderr, bytes: &outputBytes}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return processorRunFailure("start", err, outputBytes.Load(), output)
	}
	control, err := attach(cmd, ctx.Err)
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		stdout.Close()
		stderr.Close()
		return processorRunFailure("attach", err, outputBytes.Load(), output)
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
		if err := parent.Err(); err != nil {
			return err
		}
		stdout.Close()
		stderr.Close()
		return processorRunFailure("timeout", ctx.Err(), outputBytes.Load(), output)
	}
	if err := ctx.Err(); err != nil {
		if parentErr := parent.Err(); parentErr != nil {
			return parentErr
		}
		stdout.Close()
		stderr.Close()
		return processorRunFailure("timeout", err, outputBytes.Load(), output)
	}
	if err == nil {
		return nil
	}
	stdout.Close()
	stderr.Close()
	kind := "exit"
	if errors.Is(err, exec.ErrWaitDelay) {
		kind = "drain"
	}
	return processorRunFailure(kind, err, outputBytes.Load(), output)
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
