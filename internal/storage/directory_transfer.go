package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"mcl/internal/app"
	"mcl/internal/core/discovery"
	"mcl/internal/core/instance"
	"mcl/internal/core/transfer"
	"mcl/internal/platform/instancelock"
)

const directoryPending = "directory-transfer.pending.json"
const directoryBackupOwner = ".mcl-directory-backup.json"

var payloadParts = []string{"game", "removed-assets", "asset-updates"}

type DirectoryTransferStore struct{ root string }

func NewDirectoryTransferStore(root string) *DirectoryTransferStore {
	return &DirectoryTransferStore{root}
}
func (s *DirectoryTransferStore) Lock(ctx context.Context, id string) (io.Closer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return instancelock.AcquireDirectoryTransfer(s.root, id)
}
func (s *DirectoryTransferStore) central(id string) (*os.Root, error) {
	if !instance.ValidID(id) {
		return nil, errors.New("实例ID无效")
	}
	parent, err := os.OpenRoot(filepath.Join(s.root, "instances"))
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	return openRealDirectory(parent, id)
}
func instanceFingerprint(value instance.Instance) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func (s *DirectoryTransferStore) parent(ctx context.Context, location, id string) (*os.Root, error) {
	if location == "" {
		return s.central(id)
	}
	return NewGameDirectoryStore(s.root).open(ctx, location)
}
func (s *DirectoryTransferStore) payload(ctx context.Context, location, id string) (*os.Root, error) {
	parent, err := s.parent(ctx, location, id)
	if err != nil {
		return nil, err
	}
	if location == "" {
		return parent, nil
	}
	defer parent.Close()
	return openRealDirectory(parent, id)
}
func (s *DirectoryTransferStore) CheckLocation(ctx context.Context, id string) error {
	if id == "" {
		return ctx.Err()
	}
	root, err := NewGameDirectoryStore(s.root).open(ctx, id)
	if err != nil {
		return err
	}
	return root.Close()
}
func (s *DirectoryTransferStore) source(ctx context.Context, sourceID, candidateID, dataPath string) (discovery.Candidate, string, *os.Root, error) {
	var candidate discovery.Candidate
	sources, err := NewExistingGamesStore(s.root).List(ctx)
	if err != nil {
		return candidate, "", nil, err
	}
	index := slices.IndexFunc(sources, func(value app.ExistingSource) bool { return value.ID == sourceID })
	if index < 0 {
		return candidate, "", nil, errors.New("只读来源已移除")
	}
	root, err := openExistingRoot(sources[index].Path)
	if err != nil {
		return candidate, "", nil, err
	}
	defer root.Close()
	report, err := discovery.Scan(ctx, root)
	if err != nil {
		return candidate, "", nil, err
	}
	index = slices.IndexFunc(report.Candidates, func(value discovery.Candidate) bool { return value.ID == candidateID })
	if index < 0 {
		return candidate, "", nil, errors.New("来源候选已消失，请重新发现")
	}
	candidate = report.Candidates[index]
	if !slices.Contains(candidate.GamePaths, dataPath) {
		return candidate, "", nil, errors.New("请选择当前候选明确列出的共享或隔离游戏目录")
	}
	relative, err := filepath.Rel(root.Name(), dataPath)
	if err != nil {
		return candidate, "", nil, err
	}
	game, err := transfer.OpenDirectory(root, filepath.ToSlash(relative))
	if err != nil {
		return candidate, "", nil, err
	}
	data, _ := json.Marshal(candidate)
	sum := sha256.Sum256(data)
	return candidate, hex.EncodeToString(sum[:]), game, nil
}
func (s *DirectoryTransferStore) InspectSource(ctx context.Context, request app.DirectoryImportRequest, report func(transfer.Progress)) (discovery.Candidate, string, transfer.Tree, error) {
	candidate, fingerprint, game, err := s.source(ctx, request.SourceID, request.CandidateID, request.DataPath)
	if err != nil {
		return candidate, "", transfer.Tree{}, err
	}
	defer game.Close()
	target := filepath.Join(s.root, "instances")
	if request.DirectoryID != "" {
		location, err := NewGameDirectoryStore(s.root).open(ctx, request.DirectoryID)
		if err != nil {
			return candidate, "", transfer.Tree{}, err
		}
		target = location.Name()
		location.Close()
	}
	if sourcePathsOverlap(filepath.Dir(filepath.Dir(candidate.Path)), target) {
		return candidate, "", transfer.Tree{}, errors.New("源游戏资料与目标位置相互嵌套，不能复制到自身")
	}
	tree, err := transfer.Scan(ctx, game, request.Groups, true, report)
	return candidate, fingerprint, tree, err
}
func (s *DirectoryTransferStore) PlanMove(ctx context.Context, id, to string, report func(transfer.Progress)) (app.DirectoryPlan, error) {
	var plan app.DirectoryPlan
	value, err := NewInstanceStore(filepath.Join(s.root, "instances")).Get(ctx, id)
	if err != nil {
		return plan, err
	}
	if err := s.checkHistoryCapacity(ctx, id); err != nil {
		return plan, err
	}
	if value.GameDirectoryID == to {
		return plan, errors.New("目标与当前游戏位置相同")
	}
	if err := s.CheckLocation(ctx, to); err != nil {
		return plan, err
	}
	source, err := s.payload(ctx, value.GameDirectoryID, id)
	if err != nil {
		return plan, err
	}
	defer source.Close()
	if err := checkPayloadSource(ctx, source, value.ID, value.GameDirectoryID != ""); err != nil {
		return plan, err
	}
	tree, err := transfer.Scan(ctx, source, payloadParts, false, report)
	if err != nil {
		return plan, err
	}
	after := value
	after.GameDirectoryID = to
	plan = app.DirectoryPlan{Kind: "move", Token: instance.NewID(), Before: value, After: after, Tree: tree}
	if err := s.destinationFree(ctx, plan); err != nil {
		return plan, err
	}
	return plan, after.Validate()
}
func checkPayloadSource(ctx context.Context, root *os.Root, id string, external bool) error {
	if _, err := root.Lstat(directoryBackupOwner); err == nil {
		return errors.New("载荷带有未结束迁移的备份标记，请先恢复原操作")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if external {
		if err := validateExistingDirectoryOwner(ctx, root, id); err != nil {
			return err
		}
	}
	game, err := transfer.OpenDirectory(root, "game")
	if err != nil {
		return fmt.Errorf("游戏载荷不可用: %w", err)
	}
	game.Close()
	for _, part := range payloadParts {
		info, err := root.Lstat(part)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("游戏或专属备份目录不是普通目录")
		}
		node, err := openRealDirectory(root, part)
		if err != nil {
			return err
		}
		err = validateExistingDirectoryOwner(ctx, node, id)
		node.Close()
		if err != nil {
			return err
		}
	}
	if !external {
		return nil
	}
	f, err := root.Open(".")
	if err != nil {
		return err
	}
	defer f.Close()
	entries, err := f.ReadDir(32)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if len(entries) >= 32 {
		return errors.New("外部载荷有未知顶层内容，未迁移")
	}
	for _, entry := range entries {
		if !slices.Contains(payloadParts, entry.Name()) && entry.Name() != transfer.OwnerFile && entry.Name() != directoryBackupOwner && entry.Name() != ".DS_Store" {
			return errors.New("外部载荷有未纳入迁移的顶层内容，请先核对")
		}
	}
	return nil
}
func (s *DirectoryTransferStore) destinationFree(ctx context.Context, plan app.DirectoryPlan) error {
	parent, err := s.parent(ctx, plan.After.GameDirectoryID, plan.After.ID)
	if err != nil {
		return err
	}
	defer parent.Close()
	names := payloadParts
	if plan.Kind == "import" {
		names = []string{"game"}
	}
	if plan.After.GameDirectoryID != "" {
		names = []string{plan.After.ID}
	}
	for _, name := range names {
		if _, err := parent.Lstat(name); err == nil {
			return errors.New("目标已有同名载荷，未覆盖")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return ctx.Err()
}

func validateExistingDirectoryOwner(ctx context.Context, root *os.Root, id string) error {
	var owner directoryOwner
	err := readUpdateJSON(ctx, root, transfer.OwnerFile, 2048, &owner)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if owner.Schema != 1 || owner.ID != id || !instance.ValidID(owner.Token) || len(owner.Fingerprint) != 64 {
		return errors.New("载荷所属标记与实例身份不符")
	}
	return nil
}

func (s *DirectoryTransferStore) GamePath(ctx context.Context, value instance.Instance) (string, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	if value.GameDirectoryID == "" {
		return filepath.Join(s.root, "instances", value.ID, "game"), ctx.Err()
	}
	parent, err := NewGameDirectoryStore(s.root).open(ctx, value.GameDirectoryID)
	if err != nil {
		return "", err
	}
	defer parent.Close()
	return filepath.Join(parent.Name(), value.ID, "game"), ctx.Err()
}
