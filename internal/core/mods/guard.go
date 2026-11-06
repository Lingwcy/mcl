package mods

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"mcl/internal/core/archive"
)

func modFileKey(name string) string { return archive.Key(strings.TrimSuffix(name, ".disabled")) }

// GuardTransition compares the same runtime before and after an explicit edit.
// It protects previously established relationships and rejects definite errors
// introduced by edited parents/peers; unrelated old problems remain diagnosable.
// Unknown new conditions are not a claim of compatibility.
func GuardTransition(ctx context.Context, before, after []File, runtime Runtime, affected []string) error {
	old, err := check(ctx, before, runtime, false, true)
	if err != nil {
		return err
	}
	next, err := check(ctx, after, runtime, false, true)
	if err != nil {
		return err
	}
	if old.relationsTruncated || next.relationsTruncated {
		return errors.New("模组关系超过检查上限，未修改文件")
	}
	changed := make(map[string]bool)
	for _, name := range affected {
		if name != "" {
			changed[modFileKey(name)] = true
		}
	}
	previous := make(map[string]Verdict, len(old.relations))
	for _, relation := range old.relations {
		state, known := previous[relation.key]
		if !known || relation.state == Yes || state == No && relation.state == Unknown {
			previous[relation.key] = relation.state
		}
	}
	for _, relation := range next.relations {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !relation.fatal || relation.state == Yes {
			continue
		}
		state, known := previous[relation.key]
		if known && state == Yes {
			if relation.state == Unknown {
				return fmt.Errorf("无法确认仍满足已有依赖，未修改文件：%s；请先检查资产", relation.detail)
			}
			return fmt.Errorf("变更会破坏已有关系（%s）：%s", relation.reason, relation.detail)
		}
		if relation.state == No && (!known || changed[modFileKey(relation.path)] || relation.peer != "" && changed[modFileKey(relation.peer)]) {
			return fmt.Errorf("变更后的模组声明不满足要求（%s）：%s", relation.reason, relation.detail)
		}
	}
	issueKey := func(issue Issue) string {
		return issue.Code + "\x00" + modFileKey(issue.Path) + "\x00" + issue.ModID + "\x00" + issue.Detail
	}
	oldProblems := make(map[string]bool)
	for _, issue := range old.structural {
		oldProblems[issueKey(issue)] = true
	}
	for _, issue := range next.structural {
		if !oldProblems[issueKey(issue)] {
			return fmt.Errorf("变更会新增声明问题，未修改文件：%s · %s", issue.Title, issue.Detail)
		}
	}
	return ctx.Err()
}
