package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"slices"

	"mcl/internal/app"
	"mcl/internal/core/instance"
	"mcl/internal/core/transfer"
)

const packPublicationOwner = ".mcl-pack-publication.json"

type packPublication struct {
	Schema                                       int
	InstanceID, ArchiveSHA1, GameSHA1, Directory string
	World                                        string
	Loader                                       instance.Loader
}

func packPublicationFor(plan app.ArchiveImportPlan) packPublication {
	loader, _ := plan.Instance.RequiredLoader()
	return packPublication{1, plan.Instance.ID, plan.Archive.SHA1, plan.Instance.Version.SHA1, plan.Instance.GameDirectoryID, plan.Request.World, loader}
}
func readPackPublication(ctx context.Context, root *os.Root, plan app.ArchiveImportPlan) error {
	var owner packPublication
	if err := readUpdateJSON(ctx, root, packPublicationOwner, 2048, &owner); err != nil {
		return err
	}
	if owner != packPublicationFor(plan) {
		return errors.New("外部整合包载荷不属于本次固定导入，未接管")
	}
	return nil
}
func samePackPayload(ctx context.Context, prepared, published *os.Root) error {
	actual, err := transfer.Scan(ctx, prepared, []string{"game"}, false, nil)
	if err != nil {
		return err
	}
	if err := transfer.VerifyCopied(ctx, published, actual); err != nil {
		return errors.New("已发布的整合包载荷已改变，保留原文件，请先核对")
	}
	f, err := published.Open(".")
	if err != nil {
		return err
	}
	entries, err := f.ReadDir(4)
	f.Close()
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	for _, entry := range entries {
		if entry.Name() != "game" && entry.Name() != packPublicationOwner {
			return errors.New("已发布整合包出现额外文件，未接管")
		}
	}
	return nil
}
func (s *ArchiveStore) CheckImportedTask(ctx context.Context, value instance.Instance, plan app.ArchiveImportPlan) error {
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return err
	}
	defer root.Close()
	origin, _, err := readPackOrigin(root, value)
	if err != nil {
		return err
	}
	if origin.Archive == nil || origin.Archive.SHA1 != plan.Archive.SHA1 || origin.Archive.Size != plan.Archive.Size || origin.Request.Source != plan.Request.Source || origin.Request.ProjectID != plan.Request.ProjectID || origin.Request.VersionID != plan.Request.VersionID || !slices.Equal(origin.Optional, plan.SelectedOptional) {
		return errors.New("已发布整合包来源或可选项与固定任务不符")
	}
	expected := app.ArchiveImportPlan{Instance: value, Archive: *origin.Archive, Files: origin.Resolved, SelectedOptional: origin.Optional, WorldFiles: plan.WorldFiles}
	if origin.Resolved == nil {
		return errors.New("已导入整合包缺少固定文件发布证据")
	}
	if app.PackTaskIdentity(expected) != app.PackTaskIdentity(plan) {
		return errors.New("已发布整合包文件清单与固定任务不符")
	}
	return ctx.Err()
}
