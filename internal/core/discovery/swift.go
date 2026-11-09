package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"mcl/internal/core/archive"
	"mcl/internal/core/meta"
)

type SwiftIndexReader func(context.Context, []byte, string) ([]SwiftProfile, error)

// Only names below the known profiles directory are accepted. Database paths,
// commands and arguments cannot redirect reads or become launch configuration.
func SwiftCandidates(ctx context.Context, scope *os.Root, launcher string, rows []SwiftProfile) (Report, error) {
	report := Report{Root: scope.Name()}
	if len(rows) > MaxCandidates {
		return report, errors.New("Swift实例数量超过2048项，请直接选择实例目录")
	}
	names, ids := map[string]int{}, map[string]int{}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if filepath.Clean(row.WorkingPath) != launcher {
			if len(report.Problems) < MaxProblems {
				report.Problems = append(report.Problems, "Swift实例登记的工作位置与当前目录不符")
			}
			continue
		}
		if !meta.ValidID(row.Name) || !validSwiftProfileID(row.ID) {
			if len(report.Problems) < MaxProblems {
				report.Problems = append(report.Problems, "Swift实例登记包含不安全的名称或身份")
			}
			continue
		}
		directory := filepath.Join(launcher, "profiles", row.Name)
		relative, err := filepath.Rel(scope.Name(), directory)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || filepath.IsAbs(relative) {
			continue
		}
		candidate := Candidate{Format: "swift", Folder: row.Name, DocumentID: row.ID, GameVersion: row.GameVersion, MainClass: row.MainClass, Path: directory}
		sum := sha256.Sum256([]byte("swift\n" + row.ID + "\n" + directory))
		candidate.ID = hex.EncodeToString(sum[:])
		data, _ := json.Marshal(row)
		sum = sha256.Sum256(data)
		candidate.MetadataSHA256 = hex.EncodeToString(sum[:])
		candidate.DocumentDigests = []string{candidate.MetadataSHA256}
		candidate.Documents = []string{"game_versions:" + row.ID}
		if !meta.ValidID(row.GameVersion) {
			candidate.Problems = append(candidate.Problems, "实例登记缺少安全的游戏版本")
			candidate.GameVersion = ""
		}
		kind := strings.ToLower(strings.TrimSpace(row.Loader))
		switch kind {
		case "", "vanilla":
			if row.LoaderVersion != "" {
				candidate.UnknownComponents = true
				candidate.Problems = append(candidate.Problems, "实例登记的加载器与版本不一致")
			}
		case "fabric", "quilt", "forge", "neoforge":
			if !meta.ValidID(row.LoaderVersion) || strings.ContainsAny(row.LoaderVersion, " /\\:") {
				candidate.UnknownComponents = true
				candidate.Problems = append(candidate.Problems, "实例登记缺少安全的加载器版本")
			} else {
				candidate.Loaders = []Loader{{Kind: kind, Version: row.LoaderVersion}}
			}
		default:
			candidate.UnknownComponents = true
			candidate.Problems = append(candidate.Problems, "尚不支持这个实例的加载器")
		}
		if _, unknown := loaderHints(document{MainClass: row.MainClass}); unknown {
			candidate.UnknownComponents = true
			candidate.Problems = append(candidate.Problems, "实例包含未知客户端入口")
		}
		game, err := openDirectory(scope, filepath.ToSlash(relative))
		if err != nil {
			candidate.Problems = append(candidate.Problems, "实例游戏目录不可读取")
		} else {
			candidate.GamePaths = []string{directory}
			game.Close()
		}
		candidate.Status = "identified"
		if len(candidate.Problems) > 0 {
			candidate.Status = "incomplete"
		}
		duplicate := func(index int) {
			candidate.UnknownComponents = true
			candidate.Status = "incomplete"
			candidate.Problems = append(candidate.Problems, "Swift实例登记存在重复身份或目录别名")
			report.Candidates[index].UnknownComponents = true
			report.Candidates[index].Status = "incomplete"
			report.Candidates[index].Problems = append(report.Candidates[index].Problems, "Swift实例登记存在重复身份或目录别名")
		}
		if previous, ok := names[archive.Key(row.Name)]; ok {
			duplicate(previous)
		}
		if previous, ok := ids[row.ID]; ok {
			duplicate(previous)
		}
		names[archive.Key(row.Name)], ids[row.ID] = len(report.Candidates), len(report.Candidates)
		report.Candidates = append(report.Candidates, candidate)
	}
	slices.SortFunc(report.Candidates, func(a, b Candidate) int { return strings.Compare(a.Folder, b.Folder) })
	return report, ctx.Err()
}

func validSwiftProfileID(id string) bool {
	if len(id) != 36 && len(id) != 32 {
		return false
	}
	raw := strings.ReplaceAll(id, "-", "")
	if len(raw) != 32 {
		return false
	}
	_, err := hex.DecodeString(raw)
	return err == nil
}
