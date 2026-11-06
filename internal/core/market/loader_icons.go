package market

import (
	"context"
	"errors"
)

// LoaderIcons is public decoration metadata, independent of CurseForge keys.
func (c *Client) LoaderIcons(ctx context.Context) ([]LoaderIcon, error) {
	var raw []struct {
		Name string `json:"name"`
		Icon string `json:"icon"`
	}
	if err := c.get(ctx, Modrinth, "/tag/loader", nil, &raw); err != nil {
		return nil, err
	}
	if len(raw) > 128 {
		return nil, errors.New("加载器图标数量超出限制")
	}
	var result []LoaderIcon
	for _, entry := range raw {
		if ValidID(entry.Name) && len(entry.Icon) > 0 && len(entry.Icon) <= 32<<10 {
			result = append(result, LoaderIcon{Name: entry.Name, SVG: entry.Icon})
		}
	}
	return result, nil
}
