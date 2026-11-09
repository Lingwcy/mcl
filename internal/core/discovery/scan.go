package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"mcl/internal/core/archive"
	"mcl/internal/core/meta"
)

type scannedDocument struct {
	value  document
	digest string
	err    error
}
type scanner struct {
	ctx      context.Context
	versions *os.Root
	report   *Report
	cache    map[string]scannedDocument
	root     *os.Root
}

func Scan(ctx context.Context, root *os.Root) (Report, error) {
	report := Report{Root: root.Name()}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	versions, err := openDirectory(root, "versions")
	if err != nil {
		report.Problems = append(report.Problems, "无法读取普通 versions 目录："+err.Error())
		return report, ctx.Err()
	}
	defer versions.Close()
	directory, err := versions.Open(".")
	if err != nil {
		return report, err
	}
	defer directory.Close()
	entries, err := directory.ReadDir(MaxCandidates + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return report, err
	}
	if len(entries) > MaxCandidates {
		report.Truncated = true
		entries = entries[:MaxCandidates]
		report.Problems = append(report.Problems, "versions目录项超过2048，本次仅扫描已读取部分")
	}
	slices.SortFunc(entries, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	s := scanner{ctx: ctx, versions: versions, root: root, report: &report, cache: make(map[string]scannedDocument)}
	aliases := make(map[string]string)
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		folder := entry.Name()
		if !meta.ValidID(folder) {
			s.problem("已跳过不安全版本目录名称")
			continue
		}
		candidate := s.candidate(folder)
		key := archive.Key(folder)
		if previous, duplicate := aliases[key]; duplicate && previous != folder {
			candidate.Problems = append(candidate.Problems, "版本目录存在大小写或Unicode别名")
		} else {
			aliases[key] = folder
		}
		if len(candidate.Problems) > 0 {
			candidate.Status = "incomplete"
		} else {
			candidate.Status = "identified"
		}
		if candidate.DocumentID == "" {
			candidate.Status = "unreadable"
		}
		report.Candidates = append(report.Candidates, candidate)
	}
	if report.MetadataBytes >= MaxMetadataBytes {
		report.Truncated = true
		s.problem("本次元数据读取已达64MiB边界")
	}
	return report, ctx.Err()
}
func (s *scanner) problem(message string) {
	if len(s.report.Problems) < MaxProblems {
		s.report.Problems = append(s.report.Problems, message)
	} else {
		s.report.Truncated = true
	}
}
func (s *scanner) load(folder string) scannedDocument {
	if cached, ok := s.cache[folder]; ok {
		return cached
	}
	result := scannedDocument{}
	defer func() { s.cache[folder] = result }()
	if s.report.DocumentsRead >= MaxCandidates || s.report.MetadataBytes >= MaxMetadataBytes {
		s.report.Truncated = true
		result.err = errors.New("本次元数据读取数量或总量达到边界")
		return result
	}
	s.report.DocumentsRead++
	directory, err := openDirectory(s.versions, folder)
	if err != nil {
		result.err = err
		return result
	}
	defer directory.Close()
	budget := min(int64(MaxDocumentBytes), int64(MaxMetadataBytes)-s.report.MetadataBytes)
	data, err := readDocument(s.ctx, directory, folder+".json", budget, &s.report.MetadataBytes)
	if err != nil {
		result.err = err
		return result
	}
	digest := sha256.Sum256(data)
	result.digest = hex.EncodeToString(digest[:])
	result.value, result.err = parseDocument(data)
	return result
}
func (s *scanner) candidate(folder string) Candidate {
	candidate := Candidate{Folder: folder, Path: filepath.Join(s.report.Root, "versions", folder)}
	sum := sha256.Sum256([]byte(candidate.Path))
	candidate.ID = hex.EncodeToString(sum[:])
	seen := make(map[string]bool)
	current := folder
	clientFolder := ""
	note := func(message string) {
		if len(candidate.Problems) < 12 && !slices.Contains(candidate.Problems, message) {
			candidate.Problems = append(candidate.Problems, message)
		}
	}
	for depth := 0; depth < 8; depth++ {
		if err := s.ctx.Err(); err != nil {
			note("扫描已取消")
			break
		}
		if seen[current] {
			candidate.UnknownComponents = true
			note("本地父版本引用存在循环")
			break
		}
		seen[current] = true
		snapshot := s.load(current)
		if snapshot.err != nil {
			note(fmt.Sprintf("版本文档%s不可读取：%v", current, snapshot.err))
			break
		}
		doc := snapshot.value
		candidate.Documents = append(candidate.Documents, current)
		if depth == 0 {
			candidate.DocumentID, candidate.MainClass, candidate.MetadataSHA256 = doc.ID, doc.MainClass, snapshot.digest
			if doc.ID != folder {
				note("文档ID与目录名称不同；未重命名或改写")
			}
		}
		if doc.MainClass != "" && candidate.MainClass == "" {
			candidate.MainClass = doc.MainClass
		}
		candidate.DocumentDigests = append(candidate.DocumentDigests, snapshot.digest)
		hints, unknown := loaderHints(doc)
		if unknown {
			candidate.UnknownComponents = true
			note("检测到未知运行组件；不能据此确认兼容")
		}
		for _, hint := range hints {
			if !slices.Contains(candidate.Loaders, hint) {
				candidate.Loaders = append(candidate.Loaders, hint)
			}
		}
		if clientFolder == "" {
			if doc.Jar != "" {
				clientFolder = doc.Jar
			} else if file, ok := doc.Downloads["client"]; ok && file.Size > 0 {
				clientFolder = current
			} else if doc.InheritsFrom == "" {
				clientFolder = current
			}
		}
		if doc.InheritsFrom == "" {
			candidate.GameVersion = doc.ID
			if len(hints) > 0 && len(candidate.Documents) == 1 {
				candidate.GameVersion = ""
				note("游戏基底版本未由父文档明确声明")
			}
			break
		}
		candidate.GameVersion = doc.InheritsFrom
		current = doc.InheritsFrom
		if depth == 7 {
			candidate.UnknownComponents = true
			note("本地版本继承超过8层")
		}
	}
	if len(candidate.Loaders) > 1 {
		note("检测到多项加载器声明，需要逐项核对")
	}
	if candidate.MainClass == "" {
		note("版本缺少客户端主类")
	}
	if clientFolder != "" {
		candidate.ClientPath = filepath.Join(s.report.Root, "versions", clientFolder, clientFolder+".jar")
		directory, err := openDirectory(s.versions, clientFolder)
		if err == nil {
			info, fileErr := directory.Lstat(clientFolder + ".jar")
			if fileErr != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
				note("客户端JAR缺失、为空或不是普通文件")
			}
			directory.Close()
		} else {
			note("客户端JAR目录不可读取")
		}
	} else {
		note("客户端JAR归属尚未明确")
	}
	for _, shared := range []string{"libraries", "assets"} {
		directory, err := openDirectory(s.root, shared)
		if err != nil {
			note(shared + "共享目录缺失或不可读取")
		} else {
			directory.Close()
		}
	}
	// These are alternatives, not a guessed selection. Shared and isolated data
	// can coexist; D2 must confirm the actual source instead of picking by name.
	candidate.GamePaths = append(candidate.GamePaths, s.report.Root, candidate.Path)
	for _, name := range []string{".minecraft", "minecraft"} {
		directory, err := openDirectory(s.versions, folder+"/"+name)
		if err == nil {
			candidate.GamePaths = append(candidate.GamePaths, filepath.Join(candidate.Path, name))
			directory.Close()
		}
	}
	return candidate
}
