// Package diagnostics records bounded, non-secret execution evidence. It neither
// schedules work nor decides compatibility, completion, retries or cancellation.
package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"path"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"
)

const MaxRecords = 128

type Facts struct {
	Provider, Dialect, Game, Loader, LoaderVersion          string
	LauncherArch, HostArch, TargetOS, TargetArch, NativeABI string
	JavaVersion, JavaArch, JavaVM                           string
	ToolJavaVersion, ToolJavaArch, ToolJavaVM               string
	HostArchSource, Desktop, WindowObserver                 string `json:",omitempty"`
}

// Empty fields mean unobserved, not a guessed host/native ABI. No executable,
// game path, raw URL, account, argument text or arbitrary error is accepted.
func (f Facts) Validate() error {
	for _, v := range []string{f.Provider, f.Dialect, f.Game, f.Loader, f.LoaderVersion, f.LauncherArch, f.HostArch, f.TargetOS, f.TargetArch, f.NativeABI, f.JavaVersion, f.JavaArch, f.JavaVM, f.ToolJavaVersion, f.ToolJavaArch, f.ToolJavaVM, f.HostArchSource, f.Desktop, f.WindowObserver} {
		if len(v) > 128 || !utf8.ValidString(v) || strings.ContainsFunc(v, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("._+-", r)
		}) {
			return errors.New("诊断环境字段无效")
		}
	}
	return nil
}
func (f Facts) Merge(next Facts) Facts {
	old := []*string{&f.Provider, &f.Dialect, &f.Game, &f.Loader, &f.LoaderVersion, &f.LauncherArch, &f.HostArch, &f.TargetOS, &f.TargetArch, &f.NativeABI, &f.JavaVersion, &f.JavaArch, &f.JavaVM, &f.ToolJavaVersion, &f.ToolJavaArch, &f.ToolJavaVM, &f.HostArchSource, &f.Desktop, &f.WindowObserver}
	values := []string{next.Provider, next.Dialect, next.Game, next.Loader, next.LoaderVersion, next.LauncherArch, next.HostArch, next.TargetOS, next.TargetArch, next.NativeABI, next.JavaVersion, next.JavaArch, next.JavaVM, next.ToolJavaVersion, next.ToolJavaArch, next.ToolJavaVM, next.HostArchSource, next.Desktop, next.WindowObserver}
	if next.Validate() != nil {
		return f
	}
	for i, v := range values {
		if v != "" {
			*old[i] = v
		}
	}
	return f
}

type Record struct {
	ID, Parent                                uint64
	Phase, Subject, Source, Code              string
	HTTPStatus, Attempt, Step, Steps          int
	StartedAt, UpdatedAt, LastWorkAt, EndedAt time.Time
}

func (r Record) Validate() error {
	switch r.Phase {
	case "metadata", "java", "processor", "network", "quota", "rate", "lock", "verify", "publish", "backoff", "work", "installation-slot":
	default:
		return errors.New("诊断阶段无效")
	}
	if r.ID == 0 || r.Parent >= r.ID || r.StartedAt.IsZero() || r.UpdatedAt.Before(r.StartedAt) || !r.LastWorkAt.IsZero() && (r.LastWorkAt.Before(r.StartedAt) || r.LastWorkAt.After(r.UpdatedAt)) || !r.EndedAt.IsZero() && r.EndedAt.Before(r.StartedAt) || r.HTTPStatus != 0 && (r.HTTPStatus < 100 || r.HTTPStatus > 599) || r.Attempt < 0 || r.Attempt > 16 || r.Step < 0 || r.Steps < 0 || r.Steps > 10000 || r.Step > r.Steps || !SafeSubject(r.Subject) || !ValidCode(r.Code) {
		return errors.New("诊断记录无效")
	}
	switch r.Source {
	case "", "mojang", "forge", "neoforge", "fabric", "quilt", "temurin", "zulu", "mirror", "maven", "other":
	default:
		return errors.New("诊断来源类别无效")
	}
	return nil
}
func SafeSubject(s string) bool {
	return len(s) <= 320 && utf8.ValidString(s) && !strings.ContainsFunc(s, unicode.IsControl) && !strings.Contains(s, "://") && !strings.ContainsAny(s, "?@#") && !strings.ContainsRune(s, 92) && !strings.HasPrefix(s, "/") && path.Clean(s) != ".." && !strings.Contains(s, "../")
}
func ValidCode(s string) bool {
	switch s {
	case "", "unknown", "canceled", "deadline", "network", "unauthorized", "forbidden", "not-found", "rate-limited", "server", "integrity", "disk-full", "permission", "filesystem", "checkpoint", "interrupted":
		return true
	}
	return false
}

type Snapshot struct {
	Artifacts        []Artifact `json:",omitempty"`
	ArtifactsDropped uint64     `json:",omitempty"`
	Facts            Facts
	Records          []Record
	LastWorkAt       time.Time
	Code             string
	Dropped          uint64
}

func (s Snapshot) Clone() Snapshot {
	s.Records = slices.Clone(s.Records)
	s.Artifacts = slices.Clone(s.Artifacts)
	return s
}
func (s Snapshot) Validate() error {
	if err := s.Facts.Validate(); err != nil {
		return err
	}
	if len(s.Records) > MaxRecords || len(s.Artifacts) > MaxArtifacts || !ValidCode(s.Code) {
		return errors.New("诊断快照越界")
	}
	artifacts := map[artifactKey]bool{}
	for _, a := range s.Artifacts {
		if err := a.Validate(); err != nil {
			return err
		}
		if artifacts[a.key()] {
			return errors.New("诊断制品重复")
		}
		artifacts[a.key()] = true
	}
	seen := map[uint64]bool{}
	for _, r := range s.Records {
		if err := r.Validate(); err != nil {
			return err
		}
		if seen[r.ID] {
			return errors.New("诊断步骤重复")
		}
		seen[r.ID] = true
	}
	return nil
}
func (s *Snapshot) Apply(u Update) {
	if u.PublishedDirectory != "" && SafeSubject(u.PublishedDirectory) {
		for i, a := range s.Artifacts {
			if strings.HasPrefix(a.Destination, u.PublishedDirectory+"/") {
				s.Artifacts[i].Published = true
			}
		}
	}
	if u.Artifact != nil {
		s.applyArtifact(*u.Artifact)
	}
	if u.Facts != nil {
		s.Facts = s.Facts.Merge(*u.Facts)
	}
	if u.Record == nil || u.Record.Validate() != nil {
		return
	}
	r := *u.Record
	if r.LastWorkAt.After(s.LastWorkAt) {
		s.LastWorkAt = r.LastWorkAt
	}
	for i, old := range s.Records {
		if old.ID == r.ID {
			if !r.UpdatedAt.Before(old.UpdatedAt) && old.EndedAt.IsZero() {
				s.Records[i] = r
			}
			return
		}
	}
	if len(s.Records) >= MaxRecords {
		index := -1
		for i, old := range s.Records {
			if !old.EndedAt.IsZero() {
				index = i
				break
			}
		}
		s.Dropped++
		if index < 0 {
			return
		}
		s.Records = slices.Delete(s.Records, index, index+1)
	}
	s.Records = append(s.Records, r)
}
func (s *Snapshot) Finish(code string) {
	if ValidCode(code) {
		s.Code = code
	}
	now := time.Now().UTC()
	for i := range s.Records {
		if s.Records[i].EndedAt.IsZero() {
			s.Records[i].EndedAt = now
			s.Records[i].UpdatedAt = now
		}
	}
}

// Code never classifies by arbitrary exception strings or persists the exception.
func Code(err error) string {
	if err == nil {
		return ""
	}
	var tagged interface{ DiagnosticCode() string }
	if errors.As(err, &tagged) && ValidCode(tagged.DiagnosticCode()) {
		return tagged.DiagnosticCode()
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, syscall.ENOSPC):
		return "disk-full"
	case errors.Is(err, fs.ErrPermission):
		return "permission"
	}
	var n net.Error
	if errors.As(err, &n) {
		if n.Timeout() {
			return "deadline"
		}
		return "network"
	}
	var p *fs.PathError
	if errors.As(err, &p) {
		return "filesystem"
	}
	return "unknown"
}

type Failure struct {
	Kind  string
	Cause error
}

func (e *Failure) Error() string          { return e.Cause.Error() }
func (e *Failure) Unwrap() error          { return e.Cause }
func (e *Failure) DiagnosticCode() string { return e.Kind }
func Wrap(code string, err error) error {
	if err == nil {
		return nil
	}
	if !ValidCode(code) {
		code = "unknown"
	}
	return &Failure{code, err}
}
func HTTPCode(status int) string {
	switch status {
	case 401:
		return "unauthorized"
	case 403:
		return "forbidden"
	case 404, 410:
		return "not-found"
	case 408, 504:
		return "deadline"
	case 429:
		return "rate-limited"
	}
	if status >= 500 {
		return "server"
	}
	return "network"
}
func Source(address string) string {
	// Only categories are retained, never addresses/credentials/query strings.
	if strings.HasPrefix(address, "https://github.com/adoptium/") {
		return "temurin"
	}
	host := strings.Split(strings.TrimPrefix(address, "https://"), "/")[0]
	switch host {
	case "libraries.minecraft.net", "resources.download.minecraft.net", "piston-meta.mojang.com", "piston-data.mojang.com", "launchermeta.mojang.com", "launcher.mojang.com":
		return "mojang"
	case "files.minecraftforge.net", "maven.minecraftforge.net":
		return "forge"
	case "maven.neoforged.net":
		return "neoforge"
	case "meta.fabricmc.net", "maven.fabricmc.net":
		return "fabric"
	case "meta.quiltmc.org", "maven.quiltmc.org":
		return "quilt"
	case "api.adoptium.net":
		return "temurin"
	case "api.azul.com", "cdn.azul.com":
		return "zulu"
	case "repo.maven.apache.org", "repo1.maven.org":
		return "maven"
	case "bmclapi2.bangbang93.com", "mirrors.tuna.tsinghua.edu.cn":
		return "mirror"
	}
	return "other"
}

type Update struct {
	PublishedDirectory string
	Artifact           *Artifact
	Record             *Record
	Facts              *Facts
}
type observer struct {
	next atomic.Uint64
	send func(Update)
}
type observerKey struct{}
type spanKey struct{}

func WithObserver(ctx context.Context, send func(Update)) context.Context {
	if send == nil {
		return ctx
	}
	return context.WithValue(ctx, observerKey{}, &observer{send: send})
}
func Observed(ctx context.Context) bool {
	o, _ := ctx.Value(observerKey{}).(*observer)
	return o != nil
}

func SetFacts(ctx context.Context, f Facts) {
	if o, _ := ctx.Value(observerKey{}).(*observer); o != nil && f.Validate() == nil {
		o.send(Update{Facts: &f})
	}
}

type Span struct {
	mu       sync.Mutex
	o        *observer
	record   Record
	lastEmit time.Time
}

func Start(ctx context.Context, phase, subject string) (context.Context, *Span) {
	o, _ := ctx.Value(observerKey{}).(*observer)
	if o == nil {
		return ctx, &Span{}
	}
	now := time.Now().UTC()
	r := Record{ID: o.next.Add(1), Phase: phase, Subject: subject, StartedAt: now, UpdatedAt: now}
	if parent, _ := ctx.Value(spanKey{}).(*Span); parent != nil && parent.o == o {
		parent.mu.Lock()
		r.Parent = parent.record.ID
		parent.mu.Unlock()
	}
	if r.Validate() != nil {
		return ctx, &Span{}
	}
	s := &Span{o: o, record: r, lastEmit: now}
	o.send(Update{Record: &r})
	return context.WithValue(ctx, spanKey{}, s), s
}
func (s *Span) mutate(fn func(*Record), force bool) {
	if s == nil || s.o == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.record.EndedAt.IsZero() {
		return
	}
	fn(&s.record)
	s.record.UpdatedAt = time.Now().UTC()
	if !force && time.Since(s.lastEmit) < 250*time.Millisecond {
		return
	}
	r := s.record
	s.lastEmit = r.UpdatedAt
	if r.Validate() == nil {
		s.o.send(Update{Record: &r})
	}
}
func (s *Span) End()           { s.mutate(func(r *Record) { r.EndedAt = time.Now().UTC() }, true) }
func (s *Span) Fail(err error) { s.mutate(func(r *Record) { r.Code = Code(err) }, true) }
func (s *Span) Network(source string, status, attempt int) {
	s.mutate(func(r *Record) {
		r.Source = source
		r.HTTPStatus = status
		r.Attempt = attempt
		r.Code = ""
		if status >= 400 {
			r.Code = HTTPCode(status)
		}
	}, true)
}
func (s *Span) Step(index, total int) {
	s.mutate(func(r *Record) { r.Step = index; r.Steps = total }, true)
}
func (s *Span) Work() { s.mutate(func(r *Record) { r.LastWorkAt = time.Now().UTC() }, false) }
func Response(ctx context.Context, address string, status int) {
	if s, _ := ctx.Value(spanKey{}).(*Span); s != nil {
		s.mutate(func(r *Record) {
			r.Source = Source(address)
			r.HTTPStatus = status
			if status >= 400 {
				r.Code = HTTPCode(status)
			}
		}, true)
	}
}

func Work(ctx context.Context) {
	if s, _ := ctx.Value(spanKey{}).(*Span); s != nil {
		s.Work()
	}
}

func Label(phase string) string {
	switch phase {
	case "metadata":
		return "读取元数据"
	case "java":
		return "解析 Java"
	case "processor":
		return "执行安装处理器"
	case "network":
		return "等待网络"
	case "quota":
		return "等待下载名额"
	case "installation-slot":
		return "等待安装名额"
	case "rate":
		return "按设置限速"
	case "lock":
		return "等待共享目标"
	case "verify":
		return "校验"
	case "publish":
		return "发布"
	case "backoff":
		return "等待重试"
	case "work":
		return "准备文件"
	}
	return phase
}
func (s Snapshot) Current() (Record, bool) {
	// Show the latest active leaf, not a parent still awaiting its child. Several
	// leaves may be active concurrently; the full snapshot preserves all of them.
	parents := map[uint64]bool{}
	for _, r := range s.Records {
		if r.EndedAt.IsZero() {
			parents[r.Parent] = true
		}
	}
	var result Record
	for _, r := range s.Records {
		if r.EndedAt.IsZero() && !parents[r.ID] && (result.ID == 0 || r.StartedAt.After(result.StartedAt)) {
			result = r
		}
	}
	return result, result.ID != 0
}
func unknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
func (s Snapshot) Text() string {
	var b strings.Builder
	f := s.Facts
	fmt.Fprintln(&b, fmt.Sprintf("provider=%s dialect=%s minecraft=%s loader=%s/%s", unknown(f.Provider), unknown(f.Dialect), unknown(f.Game), unknown(f.Loader), unknown(f.LoaderVersion)))
	fmt.Fprintln(&b, fmt.Sprintf("launcher_arch=%s host_arch=%s target=%s/%s native_abi=%s", unknown(f.LauncherArch), unknown(f.HostArch), unknown(f.TargetOS), unknown(f.TargetArch), unknown(s.nativeHeaderSummary())))
	fmt.Fprintf(&b, "host_source=%s desktop=%s window_observer=%s\n", unknown(f.HostArchSource), unknown(f.Desktop), unknown(f.WindowObserver))
	fmt.Fprintln(&b, fmt.Sprintf("java=%s/%s/%s tool_java=%s/%s/%s", unknown(f.JavaVersion), unknown(f.JavaArch), unknown(f.JavaVM), unknown(f.ToolJavaVersion), unknown(f.ToolJavaArch), unknown(f.ToolJavaVM)))
	fmt.Fprintln(&b, fmt.Sprintf("code=%s last_work=%s dropped=%d", unknown(s.Code), stamp(s.LastWorkAt), s.Dropped))
	for _, r := range s.Records {
		end := "active"
		if !r.EndedAt.IsZero() {
			end = stamp(r.EndedAt)
		}
		fmt.Fprintln(&b, fmt.Sprintf("%d parent=%d %s %s source=%s http=%d attempt=%d step=%d/%d start=%s end=%s work=%s code=%s", r.ID, r.Parent, r.Phase, r.Subject, unknown(r.Source), r.HTTPStatus, r.Attempt, r.Step, r.Steps, stamp(r.StartedAt), end, stamp(r.LastWorkAt), unknown(r.Code)))
	}
	b.WriteString(s.artifactText())
	return b.String()
}
func stamp(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.UTC().Format(time.RFC3339Nano)
}
