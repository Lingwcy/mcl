package storage

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"mcl/internal/app"
	"mcl/internal/core/archive"
	"mcl/internal/core/instance"
	"mcl/internal/core/transfer"
	"mcl/internal/fileio"
	"mcl/internal/platform/instancelock"
)

type directoryState struct {
	Schema        int
	Plan          app.DirectoryPlan
	Phase         string
	BackupTree    *transfer.Tree
	BackupChanged bool
	BackupGone    bool
}

var errDirectoryOwner = errors.New("目录事务所属标记不符，未覆盖或回收")

type directoryOwner struct {
	Schema                 int
	ID, Token, Fingerprint string
}

func ownerFor(plan app.DirectoryPlan) directoryOwner {
	return directoryOwner{1, plan.After.ID, plan.Token, plan.Fingerprint()}
}
func writeDirectoryJSON(ctx context.Context, root *os.Root, name string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) > 32<<20 {
		return errors.New("目录事务记录超过32MiB")
	}
	if err := fileio.WriteAt(ctx, root, name, data, 0o600); err != nil {
		return err
	}
	file, err := root.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(syncErr, closeErr); err != nil {
		return err
	}
	return fileio.SyncDirectory(root)
}
func readDirectoryOwner(ctx context.Context, root *os.Root, name string, plan app.DirectoryPlan) error {
	var owner directoryOwner
	if err := readUpdateJSON(ctx, root, name, 2048, &owner); err != nil {
		return err
	}
	if owner != ownerFor(plan) {
		return errDirectoryOwner
	}
	return nil
}
func (s *DirectoryTransferStore) state(ctx context.Context, id string) (directoryState, error) {
	var state directoryState
	root, err := s.central(id)
	if err != nil {
		return state, err
	}
	defer root.Close()
	if err := readUpdateJSON(ctx, root, directoryPending, 32<<20, &state); err != nil {
		return state, err
	}
	return state, validateDirectoryState(id, state)
}
func validateDirectoryState(id string, state directoryState) error {
	p := state.Plan
	if state.Schema != 1 || p.After.ID != id || p.After.Validate() != nil || !instance.ValidID(p.Token) || p.Kind != "move" && p.Kind != "import" {
		return errors.New("目录恢复记录身份无效，已保留")
	}
	if !slices.Contains([]string{"copying", "publishing", "committed", "backing-up", "complete", "aborting"}, state.Phase) {
		return errors.New("目录恢复阶段无效")
	}
	if p.Kind == "move" && (p.Before.Validate() != nil || p.Before.ID != id || p.Before.GameDirectoryID == p.After.GameDirectoryID) {
		return errors.New("迁移快照无效")
	}
	if p.Kind == "move" {
		normalized := p.After
		normalized.GameDirectoryID = p.Before.GameDirectoryID
		if instanceFingerprint(normalized) != instanceFingerprint(p.Before) {
			return errors.New("目录迁移不得改变游戏或其他实例配置")
		}
	}
	if p.Kind == "import" && (p.After.JavaPath != "" || p.After.MaxMemoryMiB != 0 || p.After.Pack != nil) {
		return errors.New("外部资料复制不得继承私有启动配置")
	}
	if p.Kind == "import" && (!instance.ValidID(p.SourceID) || p.CandidateID == "" || len(p.SourceFingerprint) != 64 || !validExistingPath(p.DataPath)) {
		return errors.New("导入来源身份无效")
	}
	roots := p.Tree.Roots
	if p.Kind == "move" && !slices.Equal(roots, payloadParts) {
		return errors.New("迁移载荷分类无效")
	}
	if err := validateTransferTree(p.Tree, p.Kind == "import"); err != nil {
		return err
	}
	return nil
}
func validateTransferTree(tree transfer.Tree, importing bool) error {
	if len(tree.Roots) > 32 || len(tree.Files)+len(tree.Directories) > transfer.MaxEntries || tree.Bytes < 0 || tree.Bytes > transfer.MaxBytes || len(tree.Skipped) > 512 {
		return errors.New("目录事务清单越界")
	}
	roots := map[string]bool{}
	for _, root := range tree.Roots {
		if !archive.SafePath(root) || path.Base(root) != root || roots[archive.Key(root)] || importing && !slices.Contains(transfer.ImportGroups, root) {
			return errors.New("目录事务分类无效")
		}
		roots[archive.Key(root)] = true
	}
	seen := map[string]bool{}
	check := func(name string) bool {
		key := archive.Key(name)
		if !archive.SafePath(name) || seen[key] || !roots[archive.Key(strings.SplitN(name, "/", 2)[0])] {
			return false
		}
		seen[key] = true
		return true
	}
	var size int64
	for _, name := range tree.Directories {
		if !check(name) {
			return errors.New("目录事务存在重复或越界目录")
		}
	}
	for _, file := range tree.Files {
		if !check(file.Path) || file.Size < 0 || file.Size > transfer.MaxFileBytes || len(file.SHA256) != 64 {
			return errors.New("目录事务文件无效")
		}
		if _, err := hex.DecodeString(file.SHA256); err != nil {
			return errors.New("文件 SHA-256 格式无效")
		}
		size += file.Size
	}
	if size != tree.Bytes {
		return errors.New("目录事务总大小不一致")
	}
	data, err := json.Marshal(tree)
	if err != nil || len(data) > 12<<20 {
		return errors.New("目录事务清单编码超过12MiB")
	}
	return nil
}
func (s *DirectoryTransferStore) writeState(ctx context.Context, state directoryState) error {
	if err := validateDirectoryState(state.Plan.After.ID, state); err != nil {
		return err
	}
	root, err := s.central(state.Plan.After.ID)
	if err != nil {
		return err
	}
	defer root.Close()
	return writeDirectoryJSON(ctx, root, directoryPending, state)
}
func (s *DirectoryTransferStore) Pending(ctx context.Context, id string) (app.DirectoryPlan, error) {
	state, err := s.state(ctx, id)
	return state.Plan, err
}
func (s *DirectoryTransferStore) Begin(ctx context.Context, plan app.DirectoryPlan) error {
	// Registrations cannot disappear between checking the target/source and
	// publishing the persistent reference. Never hold these locks during copying.
	if plan.Kind == "import" {
		sourceLock, err := instancelock.AcquireExistingSources(s.root)
		if err != nil {
			return err
		}
		defer sourceLock.Close()
		rows, err := NewExistingGamesStore(s.root).List(ctx)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(rows, func(row app.ExistingSource) bool { return row.ID == plan.SourceID }) {
			return errors.New("导入来源已移除")
		}
	}
	locations, err := instancelock.AcquireGameDirectories(s.root)
	if err != nil {
		return err
	}
	defer locations.Close()
	state := directoryState{Schema: 1, Plan: plan, Phase: "copying"}
	if err := validateDirectoryState(plan.After.ID, state); err != nil {
		return err
	}
	existing, err := s.state(ctx, plan.After.ID)
	if err == nil {
		if existing.Plan.Fingerprint() != plan.Fingerprint() {
			return errors.New("另一个目录事务已存在，请重新读取")
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if plan.Kind == "move" {
		value, err := NewInstanceStore(filepath.Join(s.root, "instances")).Get(ctx, plan.Before.ID)
		if err != nil {
			return err
		}
		if instanceFingerprint(value) != instanceFingerprint(plan.Before) {
			return errors.New("实例已变化，请重新预览迁移")
		}
		if err := s.destinationFree(ctx, plan); err != nil {
			return err
		}
		return s.writeState(ctx, state)
	}
	if err := s.CheckLocation(ctx, plan.After.GameDirectoryID); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(s.root, "instances"), 0o700); err != nil {
		return err
	}
	parent, err := os.OpenRoot(filepath.Join(s.root, "instances"))
	if err != nil {
		return err
	}
	defer parent.Close()
	temporary := ".mcl-directory-new-" + plan.Token
	if err := parent.Mkdir(temporary, 0o700); err != nil {
		return err
	}
	// Unpublished metadata contains no gameplay files and is exclusively ours.
	defer parent.RemoveAll(temporary)
	root, err := openRealDirectory(parent, temporary)
	if err != nil {
		return err
	}
	if err := root.Mkdir("runtime", 0o700); err != nil {
		root.Close()
		return err
	}
	err = writeDirectoryJSON(ctx, root, "instance.json", instanceDocument{Version: instanceDocumentVersion(plan.After), Instance: plan.After})
	if err == nil {
		err = writeDirectoryJSON(ctx, root, directoryPending, state)
	}
	root.Close()
	if err != nil {
		return err
	}
	if err := directoryMove(parent, temporary, parent, plan.After.ID); err != nil {
		return err
	}
	return nil
}
func (s *DirectoryTransferStore) ListOperations(ctx context.Context) ([]app.DirectoryOperation, error) {
	parent, err := os.OpenRoot(filepath.Join(s.root, "instances"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	f, err := parent.Open(".")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries, err := f.ReadDir(10001)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > 10000 {
		return nil, errors.New("实例目录超过任务枚举边界")
	}
	var result []app.DirectoryOperation
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !instance.ValidID(entry.Name()) {
			continue
		}
		root, err := openRealDirectory(parent, entry.Name())
		if err != nil {
			return nil, err
		}
		if _, err := root.Lstat(directoryPending); err == nil {
			state, e := s.state(ctx, entry.Name())
			if e != nil {
				root.Close()
				return nil, e
			}
			result = append(result, directoryOperation(state, false))
		} else if !errors.Is(err, os.ErrNotExist) {
			root.Close()
			return nil, err
		}
		history, err := openRealDirectory(root, "directory-history")
		root.Close()
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		hf, err := history.Open(".")
		if err != nil {
			history.Close()
			return nil, err
		}
		records, e := hf.ReadDir(257)
		hf.Close()
		if e != nil && !errors.Is(e, io.EOF) {
			history.Close()
			return nil, e
		}
		if len(records) > 256 {
			history.Close()
			return nil, errors.New("单实例目录备份超过256条，请先清理")
		}
		for _, record := range records {
			token := strings.TrimSuffix(record.Name(), ".json")
			if !instance.ValidID(token) || record.Name() != token+".json" {
				continue
			}
			var state directoryState
			if err := readUpdateJSON(ctx, history, record.Name(), 32<<20, &state); err != nil {
				history.Close()
				return nil, err
			}
			if err := validateDirectoryState(entry.Name(), state); err != nil {
				history.Close()
				return nil, err
			}
			if !state.BackupGone {
				result = append(result, directoryOperation(state, true))
			}
		}
		history.Close()
	}
	return result, ctx.Err()
}
func directoryOperation(state directoryState, backup bool) app.DirectoryOperation {
	p := state.Plan
	return app.DirectoryOperation{SourceID: p.SourceID, InstanceID: p.After.ID, Name: p.After.Name, Kind: p.Kind, Token: p.Token, Phase: state.Phase, From: p.Before.GameDirectoryID, To: p.After.GameDirectoryID, Bytes: p.Tree.Bytes, Backup: backup, Changed: state.BackupChanged}
}
