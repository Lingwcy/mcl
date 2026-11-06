package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"slices"
	"strconv"
	"time"

	"mcl/internal/app"
	"mcl/internal/core/archive"
	"mcl/internal/core/download"
	"mcl/internal/core/instance"
	"mcl/internal/core/market"
	"mcl/internal/fileio"
)

type updateJournal struct {
	Schema       int               `json:"schema"`
	ID           string            `json:"id"`
	State        string            `json:"state"`
	CreatedAt    time.Time         `json:"createdAt"`
	Plan         market.UpdatePlan `json:"plan"`
	BeforeSHA256 string            `json:"beforeSha256"`
	AfterSHA256  string            `json:"afterSha256"`
}

func updatesRoot(managed *os.Root, create bool) (*os.Root, error) {
	if create {
		if err := managed.Mkdir("asset-updates", 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
	}
	info, err := managed.Lstat("asset-updates")
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("更新记录目录不是普通目录")
	}
	return managed.OpenRoot("asset-updates")
}

func writeUpdateJournal(ctx context.Context, transaction *os.Root, journal updateJournal) error {
	data, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	if len(data) > maxUpdateJournalSize {
		return errors.New("更新日志超出大小上限")
	}
	return fileio.WriteAt(ctx, transaction, "journal.json", data, 0o600)
}

func clearUpdatePending(ctx context.Context, managed *os.Root, id string) error {
	pending, err := readUpdatePending(ctx, managed)
	if err != nil {
		return err
	}
	if pending.ID != id {
		return errors.New("待恢复记录已改变，未清理")
	}
	return managed.Remove(updatePendingFile)
}

func plainEntry(root *os.Root, name string) (bool, error) {
	info, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, errors.New("更新文件不是普通文件")
	}
	return true, nil
}

func (s *AssetUpdateStore) CommitUpdate(ctx context.Context, plan market.UpdatePlan, report func(app.AssetUpdateProgress)) (result app.AssetUpdateResult, err error) {
	if len(plan.Changes) == 0 {
		return result, errors.New("没有需要应用的更新")
	}
	if err = s.CheckUpdate(ctx, plan); err != nil {
		return result, err
	}
	id := plan.Request.InstanceID
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return result, err
	}
	defer root.Close()
	managed, err := root.OpenRoot(path.Join("instances", id))
	if err != nil {
		return result, err
	}
	defer managed.Close()
	if _, checkErr := managed.Lstat(updatePendingFile); checkErr == nil {
		return result, errors.New("此实例已有未恢复的更新")
	} else if !errors.Is(checkErr, os.ErrNotExist) {
		return result, checkErr
	}
	game, err := s.assets.openGame(ctx, id)
	if err != nil {
		return result, err
	}
	defer game.Close()
	parent, err := updatesRoot(managed, true)
	if err != nil {
		return result, err
	}
	defer parent.Close()
	result.ID = instance.NewID()
	if err = parent.Mkdir(result.ID, 0o700); err != nil {
		return result, err
	}
	started, committed := false, false
	defer func() {
		if !started {
			parent.RemoveAll(result.ID)
		}
	}()
	transaction, err := parent.OpenRoot(result.ID)
	if err != nil {
		return result, err
	}
	defer transaction.Close()
	for _, directory := range []string{"new", "old"} {
		if err = transaction.Mkdir(directory, 0o700); err != nil {
			return result, err
		}
	}
	fresh, err := transaction.OpenRoot("new")
	if err != nil {
		return result, err
	}
	defer fresh.Close()
	old, err := transaction.OpenRoot("old")
	if err != nil {
		return result, err
	}
	defer old.Close()
	directory, _, _ := market.AssetDirectory(plan.Request.Kind, plan.Request.World)
	if err = game.MkdirAll(directory, 0o700); err != nil {
		return result, err
	}
	checked, err := localDirectory(game, directory)
	if err != nil {
		return result, err
	}
	checked.Close()
	assets, err := game.OpenRoot(directory)
	if err != nil {
		return result, err
	}
	defer assets.Close()
	before, err := readMarketReceiptAt(ctx, managed, "market.json")
	if err != nil {
		return result, err
	}
	beforeHash, err := receiptDigest(before)
	if err != nil {
		return result, err
	}
	if beforeHash != plan.ReceiptSHA256 {
		return result, errors.New("更新来源记录已改变")
	}
	after := marketReceipt{Schema: 1, Files: slices.Clone(before.Files)}
	for _, change := range plan.Changes {
		if change.Before != nil {
			key := archive.Key(change.Before.File.Path)
			after.Files = slices.DeleteFunc(after.Files, func(file market.InstallFile) bool { return archive.Key(file.Path) == key })
		}
	}
	for _, change := range plan.Changes {
		after.Files = append(after.Files, change.After)
	}
	if err = writeMarketReceiptAt(ctx, transaction, "before.json", before); err != nil {
		return result, err
	}
	if err = writeMarketReceiptAt(ctx, transaction, "after.json", after); err != nil {
		return result, err
	}
	afterHash, err := receiptDigest(after)
	if err != nil {
		return result, err
	}
	journal := updateJournal{Schema: 1, ID: result.ID, State: "prepared", CreatedAt: time.Now().UTC(), Plan: plan, BeforeSHA256: beforeHash, AfterSHA256: afterHash}
	var modEdits []localModEdit
	for index, change := range plan.Changes {
		if report != nil {
			report(app.AssetUpdateProgress{Stage: "update-stage", Progress: download.Progress{Files: index, TotalFiles: len(plan.Changes)}})
		}
		slot := strconv.Itoa(index)
		file := change.After.File
		if change.ReusesContent() {
			err = copyVerifiedAt(ctx, game, change.Before.Path, fresh, slot, file.SHA1, "", file.Size)
		} else {
			err = copyVerifiedCache(ctx, root, fresh, slot, file.SHA1, "", file.Size)
		}
		if err != nil {
			return result, err
		}
		if plan.Request.Kind == market.Mod {
			edit := localModEdit{after: change.Path(), enabled: change.Enabled, root: fresh, source: slot, size: file.Size, sha1: file.SHA1}
			if change.Before != nil {
				edit.before = change.Before.Path
			}
			modEdits = append(modEdits, edit)
		}
	}
	if len(modEdits) > 0 {
		if report != nil {
			report(app.AssetUpdateProgress{Stage: "update-local-check"})
		}
		if err = s.assets.guardLocalMods(ctx, game, id, plan.Request.Loader, modEdits); err != nil {
			return result, err
		}
	}
	if err = writeUpdateJournal(ctx, transaction, journal); err != nil {
		return result, err
	}
	marker, _ := json.Marshal(updatePending{Schema: 1, ID: result.ID})
	if err = fileio.WriteAt(ctx, managed, updatePendingFile, marker, 0o600); err != nil {
		return result, err
	}
	started = true
	defer func() {
		if err == nil || committed {
			return
		}
		recovery, stop := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer stop()
		if rollbackErr := rollbackUpdate(recovery, managed, assets, transaction, fresh, old, &journal); rollbackErr != nil {
			result.RecoveryRequired = true
			err = errors.Join(err, fmt.Errorf("旧文件仍保留在更新备份中，需恢复事务 %s: %w", result.ID, rollbackErr))
		} else {
			err = fmt.Errorf("更新未应用，已恢复旧文件: %w", err)
		}
	}()
	journal.State = "applying"
	if err = writeUpdateJournal(ctx, transaction, journal); err != nil {
		return result, err
	}
	for index, change := range plan.Changes {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		if change.Before == nil {
			continue
		}
		if err = verifyLocalAsset(ctx, assets, path.Base(change.Before.Path), change.Before.File.File.SHA1, change.Before.File.File.Size); err != nil {
			return result, err
		}
		if err = fileio.MoveNoReplace(assets, path.Base(change.Before.Path), old, strconv.Itoa(index)); err != nil {
			return result, err
		}
	}
	for index, change := range plan.Changes {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		if report != nil {
			report(app.AssetUpdateProgress{Stage: "update-apply", Progress: download.Progress{Files: index, TotalFiles: len(plan.Changes)}})
		}
		if err = fileio.MoveNoReplace(fresh, strconv.Itoa(index), assets, path.Base(change.Path())); err != nil {
			return result, err
		}
	}
	if err = writeMarketReceiptAt(ctx, managed, "market.json", after); err != nil {
		return result, err
	}
	// The receipt is the data commit point. Cancellation after this point must
	// finish the journal instead of claiming the already-applied update canceled.
	finish, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer stop()
	journal.State = "committed"
	if err = writeUpdateJournal(finish, transaction, journal); err != nil {
		return result, err
	}
	committed, result.Applied = true, true
	if err = clearUpdatePending(finish, managed, result.ID); err != nil {
		result.RecoveryRequired = true
		return result, fmt.Errorf("文件已更新，完成标记尚需恢复处理: %w", err)
	}
	return result, nil
}

func transactionReceipts(ctx context.Context, transaction *os.Root, journal updateJournal) (marketReceipt, marketReceipt, error) {
	for _, name := range []string{"before.json", "after.json"} {
		exists, err := plainEntry(transaction, name)
		if err != nil {
			return marketReceipt{}, marketReceipt{}, err
		}
		if !exists {
			return marketReceipt{}, marketReceipt{}, errors.New("更新来源快照缺失")
		}
	}
	before, err := readMarketReceiptAt(ctx, transaction, "before.json")
	if err != nil {
		return before, marketReceipt{}, err
	}
	after, err := readMarketReceiptAt(ctx, transaction, "after.json")
	if err != nil {
		return before, after, err
	}
	b, err := receiptDigest(before)
	if err != nil {
		return before, after, err
	}
	a, err := receiptDigest(after)
	if err != nil {
		return before, after, err
	}
	if b != journal.BeforeSHA256 || a != journal.AfterSHA256 || b != journal.Plan.ReceiptSHA256 {
		return before, after, errors.New("更新来源快照摘要不匹配")
	}
	return before, after, nil
}

func rollbackUpdate(ctx context.Context, managed, assets, transaction, fresh, old *os.Root, journal *updateJournal) error {
	before, _, err := transactionReceipts(ctx, transaction, *journal)
	if err != nil {
		return err
	}
	current, err := readMarketReceiptAt(ctx, managed, "market.json")
	if err != nil {
		return err
	}
	digest, err := receiptDigest(current)
	if err != nil {
		return err
	}
	if digest != journal.BeforeSHA256 && digest != journal.AfterSHA256 {
		return errors.New("来源记录被其他程序修改，未覆盖")
	}
	for index, change := range journal.Plan.Changes {
		slot := strconv.Itoa(index)
		staged, err := plainEntry(fresh, slot)
		if err != nil {
			return err
		}
		present, err := plainEntry(assets, path.Base(change.Path()))
		if err != nil {
			return err
		}
		if staged {
			if present {
				original := false
				for _, candidate := range journal.Plan.Changes {
					if candidate.Before != nil && candidate.Before.Path == change.Path() {
						if err := verifyLocalAsset(ctx, assets, path.Base(candidate.Before.Path), candidate.Before.File.File.SHA1, candidate.Before.File.File.Size); err == nil {
							original = true
						}
					}
				}
				if !original {
					return errors.New("更新目标存在非本事务发布的文件，已保留并停止恢复")
				}
			}
			continue
		}
		if !present {
			continue
		}
		if err := verifyLocalAsset(ctx, assets, path.Base(change.Path()), change.After.File.SHA1, change.After.File.Size); err != nil {
			return fmt.Errorf("新文件被修改，未移动: %w", err)
		}
		if err := fileio.MoveNoReplace(assets, path.Base(change.Path()), fresh, slot); err != nil {
			return err
		}
	}
	for index, change := range journal.Plan.Changes {
		if change.Before == nil {
			continue
		}
		slot := strconv.Itoa(index)
		saved, err := plainEntry(old, slot)
		if err != nil {
			return err
		}
		if !saved {
			if err := verifyLocalAsset(ctx, assets, path.Base(change.Before.Path), change.Before.File.File.SHA1, change.Before.File.File.Size); err != nil {
				return fmt.Errorf("旧版本文件与备份均不可用: %w", err)
			}
			continue
		}
		if err := verifyLocalAsset(ctx, old, slot, change.Before.File.File.SHA1, change.Before.File.File.Size); err != nil {
			return err
		}
		if err := fileio.MoveNoReplace(old, slot, assets, path.Base(change.Before.Path)); err != nil {
			return fmt.Errorf("旧版本目标已有文件，未覆盖: %w", err)
		}
	}
	if err := writeMarketReceiptAt(ctx, managed, "market.json", before); err != nil {
		return err
	}
	journal.State = "rolled-back"
	if err := writeUpdateJournal(ctx, transaction, *journal); err != nil {
		return err
	}
	return clearUpdatePending(ctx, managed, journal.ID)
}

func (s *AssetUpdateStore) RecoverUpdate(ctx context.Context, id, transactionID string) (app.AssetUpdateResult, error) {
	result := app.AssetUpdateResult{ID: transactionID, RecoveryRequired: true}
	if !instance.ValidID(id) || !instance.ValidID(transactionID) {
		return result, errors.New("更新恢复标识无效")
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return result, err
	}
	defer root.Close()
	managed, err := root.OpenRoot(path.Join("instances", id))
	if err != nil {
		return result, err
	}
	defer managed.Close()
	pending, err := readUpdatePending(ctx, managed)
	if err != nil {
		return result, err
	}
	if pending.ID != transactionID {
		return result, errors.New("待恢复事务已改变")
	}
	parent, err := updatesRoot(managed, false)
	if err != nil {
		return result, err
	}
	defer parent.Close()
	info, err := parent.Lstat(transactionID)
	if err != nil {
		return result, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return result, errors.New("事务目录无效")
	}
	transaction, err := parent.OpenRoot(transactionID)
	if err != nil {
		return result, err
	}
	defer transaction.Close()
	var journal updateJournal
	if err := readUpdateJSON(ctx, transaction, "journal.json", maxUpdateJournalSize, &journal); err != nil {
		return result, err
	}
	if journal.Schema != 1 || journal.ID != transactionID || journal.Plan.Request.InstanceID != id || journal.CreatedAt.IsZero() {
		return result, errors.New("更新日志身份无效")
	}
	if err := journal.Plan.Validate(); err != nil {
		return result, err
	}
	if journal.State != "prepared" && journal.State != "applying" && journal.State != "committed" && journal.State != "rolled-back" {
		return result, errors.New("更新日志状态无效")
	}
	_, after, err := transactionReceipts(ctx, transaction, journal)
	if err != nil {
		return result, err
	}
	game, err := s.assets.openGame(ctx, id)
	if err != nil {
		return result, err
	}
	defer game.Close()
	directory, _, _ := market.AssetDirectory(journal.Plan.Request.Kind, journal.Plan.Request.World)
	checked, err := localDirectory(game, directory)
	if err != nil {
		return result, err
	}
	checked.Close()
	assets, err := game.OpenRoot(directory)
	if err != nil {
		return result, err
	}
	defer assets.Close()
	if journal.State == "committed" {
		current, err := readMarketReceiptAt(ctx, managed, "market.json")
		if err != nil {
			return result, err
		}
		currentHash, _ := receiptDigest(current)
		expectedHash, _ := receiptDigest(after)
		if currentHash != expectedHash {
			return result, errors.New("已提交更新的来源记录发生变化")
		}
		for _, change := range journal.Plan.Changes {
			if err := verifyLocalAsset(ctx, assets, path.Base(change.Path()), change.After.File.SHA1, change.After.File.Size); err != nil {
				return result, err
			}
		}
		if err := clearUpdatePending(ctx, managed, journal.ID); err != nil {
			return result, err
		}
		result.Applied, result.RecoveryRequired = true, false
		return result, nil
	}
	fresh, err := transaction.OpenRoot("new")
	if err != nil {
		return result, err
	}
	defer fresh.Close()
	old, err := transaction.OpenRoot("old")
	if err != nil {
		return result, err
	}
	defer old.Close()
	if err := rollbackUpdate(ctx, managed, assets, transaction, fresh, old, &journal); err != nil {
		return result, err
	}
	result.RecoveryRequired = false
	return result, nil
}
