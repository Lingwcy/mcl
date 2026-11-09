package storage

import (
	"context"
	"errors"
	"os"
	"path"
	"slices"

	"mcl/internal/app"
	"mcl/internal/core/market"
)

// TaskUpdateStatus reports only an exact journal or exact installed desired
// result. It cannot recover a different pending transaction for the same game.
func (s *AssetUpdateStore) TaskUpdateStatus(ctx context.Context, plan market.UpdatePlan) (string, bool, error) {
	id := plan.Request.InstanceID
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return "", false, err
	}
	defer root.Close()
	managed, err := openRealDirectory(root, path.Join("instances", id))
	if err != nil {
		return "", false, err
	}
	defer managed.Close()
	pending, err := readUpdatePending(ctx, managed)
	if err == nil {
		payload, err := s.assets.openPayload(ctx, id)
		if err != nil {
			return "", false, err
		}
		defer payload.Close()
		updates, err := updatesRoot(payload, false)
		if err != nil {
			return "", false, err
		}
		defer updates.Close()
		transaction, err := openRealDirectory(updates, pending.ID)
		if err != nil {
			return "", false, err
		}
		defer transaction.Close()
		var journal updateJournal
		if err := readUpdateJSON(ctx, transaction, "journal.json", maxUpdateJournalSize, &journal); err != nil {
			return "", false, err
		}
		if journal.Schema != 1 || journal.ID != pending.ID || app.UpdateTaskIdentity(journal.Plan) != app.UpdateTaskIdentity(plan) {
			return "", false, errors.New("待恢复更新不属于此固定任务，请先在实例资产页核对")
		}
		return pending.ID, false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", false, err
	}
	if len(plan.Changes) == 0 {
		return "", false, nil
	}
	receipt, err := readMarketReceipt(ctx, root, id)
	if err != nil {
		return "", false, err
	}
	game, err := s.assets.openGame(ctx, id)
	if err != nil {
		return "", false, err
	}
	defer game.Close()
	for _, change := range plan.Changes {
		after := change.After
		if !slices.ContainsFunc(receipt.Files, func(file market.InstallFile) bool {
			return file.Source == after.Source && file.ProjectID == after.ProjectID && file.VersionID == after.VersionID && file.Path == after.Path && file.File.SHA1 == after.File.SHA1 && file.File.Size == after.File.Size
		}) {
			return "", false, nil
		}
		if err := verifyLocalAsset(ctx, game, change.Path(), after.File.SHA1, after.File.Size); err != nil {
			if ctx.Err() != nil {
				return "", false, ctx.Err()
			}
			return "", false, nil
		}
		other := after.Path + ".disabled"
		if !change.Enabled {
			other = after.Path
		}
		if _, err := game.Lstat(other); err == nil {
			return "", false, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", false, err
		}
	}
	return "", true, ctx.Err()
}
