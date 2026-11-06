package market

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

func classID(kind Kind) int {
	switch kind {
	case Modpack:
		return 4471
	case ResourcePack:
		return 12
	case Shader:
		return 6552
	case DataPack:
		return 6945
	case World:
		return 17
	default:
		return 6
	}
}
func loaderID(loader string) int {
	return map[string]int{"forge": 1, "liteloader": 3, "fabric": 4, "quilt": 5, "neoforge": 6}[loader]
}

type cfProject struct {
	AllowModDistribution *bool  `json:"allowModDistribution"`
	ClassID              int    `json:"classId"`
	ID                   int64  `json:"id"`
	Slug                 string `json:"slug"`
	Name                 string `json:"name"`
	Summary              string `json:"summary"`
	Downloads            int64  `json:"downloadCount"`
	Logo                 struct {
		URL string `json:"thumbnailUrl"`
	} `json:"logo"`
	Links struct {
		Website string `json:"websiteUrl"`
	} `json:"links"`
	Authors []struct {
		Name string `json:"name"`
	} `json:"authors"`
	Categories []struct {
		Name string `json:"name"`
	} `json:"categories"`
	LatestFilesIndexes []struct {
		Loader int `json:"modLoader"`
	} `json:"latestFilesIndexes"`
}

func (raw cfProject) project(kind Kind) Project {
	for _, candidate := range []Kind{Mod, Modpack, ResourcePack, Shader, DataPack, World} {
		if raw.ClassID == classID(candidate) {
			kind = candidate
			break
		}
	}
	p := Project{Source: CurseForge, Kind: kind, ID: strconv.FormatInt(raw.ID, 10), Slug: raw.Slug, Title: raw.Name, Description: raw.Summary, Downloads: raw.Downloads, IconURL: raw.Logo.URL, PageURL: raw.Links.Website}
	p.DistributionRestricted = raw.AllowModDistribution != nil && !*raw.AllowModDistribution
	for _, a := range raw.Authors {
		if p.Author != "" {
			p.Author += ", "
		}
		p.Author += a.Name
	}
	for _, c := range raw.Categories {
		p.Categories = append(p.Categories, c.Name)
	}
	for _, f := range raw.LatestFilesIndexes {
		tag := map[int]string{1: "forge", 3: "liteloader", 4: "fabric", 5: "quilt", 6: "neoforge"}[f.Loader]
		if tag != "" && !slices.Contains(p.Loaders, tag) {
			p.Loaders = append(p.Loaders, tag)
		}
	}
	return p
}
func (c *Client) searchCurseForge(ctx context.Context, q Query) (Results, error) {
	sort := map[string]string{"relevance": "1", "popular": "2", "downloads": "6", "updated": "3", "newest": "11", "name": "4", "follows": "2"}[q.Sort]
	if sort == "" {
		sort = "2"
	}
	values := url.Values{"gameId": {"432"}, "classId": {strconv.Itoa(classID(q.Kind))}, "sortField": {sort}, "sortOrder": {"desc"}, "index": {strconv.Itoa(q.Page * PageSize)}, "pageSize": {strconv.Itoa(PageSize)}, "searchFilter": {q.Text}}
	if q.Sort == "name" {
		values.Set("sortOrder", "asc")
	}
	if q.GameVersion != "" {
		values.Set("gameVersion", q.GameVersion)
	}
	if len(q.Categories) > 0 {
		ids := make([]int64, 0, len(q.Categories))
		for _, category := range q.Categories {
			id, _ := strconv.ParseInt(category, 10, 64)
			ids = append(ids, id)
		}
		encoded, _ := json.Marshal(ids)
		values.Set("categoryIds", string(encoded))
	}
	if len(q.Loaders) > 0 {
		ids := make([]int, 0, len(q.Loaders))
		for _, loader := range q.Loaders {
			ids = append(ids, loaderID(loader))
		}
		encoded, _ := json.Marshal(ids)
		values.Set("modLoaderTypes", string(encoded))
	}
	var raw struct {
		Data       []cfProject `json:"data"`
		Pagination struct {
			Total int `json:"totalCount"`
			Index int `json:"index"`
		} `json:"pagination"`
	}
	if err := c.get(ctx, CurseForge, "/mods/search", values, &raw); err != nil {
		return Results{}, err
	}
	result := Results{Total: min(10000, max(0, raw.Pagination.Total)), Offset: raw.Pagination.Index}
	for _, p := range raw.Data {
		result.Projects = append(result.Projects, p.project(q.Kind))
	}
	return result, nil
}
func (c *Client) categoriesCurseForge(ctx context.Context, kind Kind) ([]Category, error) {
	var raw struct {
		Data []struct {
			ID       int    `json:"id"`
			Name     string `json:"name"`
			ClassID  int    `json:"classId"`
			IsClass  bool   `json:"isClass"`
			ParentID int    `json:"parentCategoryId"`
			IconURL  string `json:"iconUrl"`
		} `json:"data"`
	}
	if err := c.get(ctx, CurseForge, "/categories", url.Values{"gameId": {"432"}, "classId": {strconv.Itoa(classID(kind))}}, &raw); err != nil {
		return nil, err
	}
	if len(raw.Data) > 2000 {
		return nil, errors.New("市场类别数量超出限制")
	}
	var result []Category
	names := make(map[int]string, len(raw.Data))
	for _, entry := range raw.Data {
		if !entry.IsClass {
			names[entry.ID] = entry.Name
		}
	}
	for _, v := range raw.Data {
		if !v.IsClass && (v.ClassID == classID(kind) || v.ClassID == 0) {
			icon := v.IconURL
			if len(icon) > 2048 || !SafeURL(icon) {
				icon = ""
			}
			result = append(result, Category{ID: strconv.Itoa(v.ID), Name: v.Name, Header: names[v.ParentID], IconURL: icon})
		}
	}
	return result, nil
}

type cfFile struct {
	Fingerprint  uint32    `json:"fileFingerprint"`
	ID           int64     `json:"id"`
	ModID        int64     `json:"modId"`
	DisplayName  string    `json:"displayName"`
	Filename     string    `json:"fileName"`
	URL          string    `json:"downloadUrl"`
	Size         int64     `json:"fileLength"`
	ReleaseType  int       `json:"releaseType"`
	Published    time.Time `json:"fileDate"`
	Downloads    int64     `json:"downloadCount"`
	GameVersions []string  `json:"gameVersions"`
	Hashes       []struct {
		Value string `json:"value"`
		Algo  int    `json:"algo"`
	} `json:"hashes"`
	Dependencies []struct {
		ID       int64 `json:"modId"`
		Relation int   `json:"relationType"`
	} `json:"dependencies"`
}

func (raw cfFile) version() Version {
	v := Version{Source: CurseForge, ID: strconv.FormatInt(raw.ID, 10), ProjectID: strconv.FormatInt(raw.ModID, 10), Name: raw.DisplayName, Number: raw.Filename, Type: map[int]string{1: "release", 2: "beta", 3: "alpha"}[raw.ReleaseType], Published: raw.Published, Downloads: raw.Downloads}
	for _, g := range raw.GameVersions {
		lower := strings.ToLower(g)
		if loaderID(lower) != 0 {
			v.Loaders = append(v.Loaders, lower)
		} else if len(g) > 0 && g[0] >= '0' && g[0] <= '9' {
			v.GameVersions = append(v.GameVersions, g)
		}
	}
	f := File{URL: raw.URL, Filename: raw.Filename, Size: raw.Size, Primary: true}
	for _, h := range raw.Hashes {
		if h.Algo == 1 {
			f.SHA1 = h.Value
		}
	}
	// A missing official URL means distribution is unavailable. Do not invent
	// CDN URLs to bypass the author's CurseForge distribution choice.
	v.Files = []File{f}
	for _, d := range raw.Dependencies {
		v.Dependencies = append(v.Dependencies, Dependency{ProjectID: strconv.FormatInt(d.ID, 10), Required: d.Relation == 3, Incompatible: d.Relation == 5})
	}
	return v
}
func (c *Client) versionsCurseForge(ctx context.Context, id, game, loader string) ([]Version, error) {
	values := url.Values{"pageSize": {"50"}}
	if game != "" {
		values.Set("gameVersion", game)
	}
	if loader != "" {
		if n := loaderID(loader); n != 0 {
			values.Set("modLoaderType", strconv.Itoa(n))
		} else {
			return nil, errors.New("不支持此加载器筛选")
		}
	}
	var result []Version
	for page := 0; page < 20; page++ {
		values.Set("index", strconv.Itoa(page*50))
		var raw struct {
			Data       []cfFile `json:"data"`
			Pagination struct {
				Total int `json:"totalCount"`
			} `json:"pagination"`
		}
		if err := c.get(ctx, CurseForge, "/mods/"+id+"/files", values, &raw); err != nil {
			return nil, err
		}
		for _, f := range raw.Data {
			v := f.version()
			if v.ProjectID == id {
				result = append(result, v)
			}
		}
		if len(raw.Data) < 50 || (page+1)*50 >= raw.Pagination.Total {
			break
		}
	}
	slices.SortStableFunc(result, func(a, b Version) int { return b.Published.Compare(a.Published) })
	return result, nil
}
