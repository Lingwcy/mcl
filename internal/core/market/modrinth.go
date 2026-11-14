package market

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"time"
)

var knownLoaders = []string{"fabric", "forge", "neoforge", "quilt", "liteloader", "rift", "babric", "legacy-fabric", "ornithe", "java-agent", "minecraft", "datapack", "iris", "optifine", "canvas", "vanilla", "bukkit", "spigot", "paper", "purpur", "folia", "sponge", "velocity"}

type mrProject struct {
	ID                string   `json:"id"`
	ProjectID         string   `json:"project_id"`
	ProjectType       string   `json:"project_type"`
	Slug              string   `json:"slug"`
	Title             string   `json:"title"`
	Description       string   `json:"description"`
	Author            string   `json:"author"`
	IconURL           string   `json:"icon_url"`
	Body              string   `json:"body"`
	Categories        []string `json:"categories"`
	DisplayCategories []string `json:"display_categories"`
	Loaders           []string `json:"loaders"`
	Downloads         int64    `json:"downloads"`
	Follows           int64    `json:"follows"`
}

func (raw mrProject) project(kind Kind) Project {
	if kind == "" {
		for _, candidate := range []Kind{Mod, Modpack, ResourcePack, Shader, DataPack} {
			if raw.ProjectType == projectType(candidate) {
				kind = candidate
				break
			}
		}
	}
	id := raw.ID
	if id == "" {
		id = raw.ProjectID
	}
	body := raw.Body
	if body != "" {
		body = NormalizeMarkdown(body)
	}
	p := Project{Source: Modrinth, Kind: kind, ID: id, Slug: raw.Slug, Title: raw.Title, Description: raw.Description, Author: raw.Author, IconURL: raw.IconURL, Body: body, Downloads: raw.Downloads, Follows: raw.Follows, Loaders: raw.Loaders}
	tags := raw.DisplayCategories
	if len(tags) == 0 {
		tags = raw.Categories
	}
	for _, tag := range tags {
		if slices.Contains(knownLoaders, tag) {
			if !slices.Contains(p.Loaders, tag) {
				p.Loaders = append(p.Loaders, tag)
			}
		} else {
			p.Categories = append(p.Categories, tag)
		}
	}
	section := projectType(kind)
	if section == "" {
		section = "project"
	}
	p.PageURL = "https://modrinth.com/" + section + "/" + url.PathEscape(id)
	return p
}
func (c *Client) searchModrinth(ctx context.Context, q Query) (Results, error) {
	facets := [][]string{{"project_type:" + projectType(q.Kind)}}
	if q.GameVersion != "" {
		facets = append(facets, []string{"versions:" + q.GameVersion})
	}
	if len(q.Loaders) > 0 {
		any := make([]string, 0, len(q.Loaders))
		for _, loader := range q.Loaders {
			any = append(any, "categories:"+loader)
		}
		facets = append(facets, any)
	}
	for _, category := range q.Categories {
		facets = append(facets, []string{"categories:" + category})
	}
	for _, environment := range q.Environments {
		facets = append(facets, environmentFacets(environment))
	}
	if q.OpenSource {
		facets = append(facets, []string{"open_source:true"})
	}
	encoded, _ := json.Marshal(facets)
	index := q.Sort
	if index == "" || index == "popular" {
		index = "relevance"
	}
	if index == "name" {
		index = "relevance"
	}
	values := url.Values{"query": {q.Text}, "facets": {string(encoded)}, "index": {index}, "limit": {strconv.Itoa(PageSize)}, "offset": {strconv.Itoa(q.Page * PageSize)}}
	var raw struct {
		Hits   []mrProject `json:"hits"`
		Total  int         `json:"total_hits"`
		Offset int         `json:"offset"`
	}
	if err := c.get(ctx, Modrinth, "/search", values, &raw); err != nil {
		return Results{}, err
	}
	result := Results{Total: max(0, raw.Total), Offset: raw.Offset}
	for _, hit := range raw.Hits {
		p := hit.project(q.Kind)
		if ValidID(p.ID) {
			result.Projects = append(result.Projects, p)
		}
	}
	return result, nil
}

// Current environments replace deprecated client_side/server_side facets.
// Selecting both sides adds two AND clauses, retaining projects where either
// required or optional support exists on each selected side.
func environmentFacets(side string) []string {
	values := []string{"client_and_server", "client_only_server_optional", "server_only_client_optional", "client_or_server", "client_or_server_prefers_both"}
	if side == "client" {
		values = append(values, "client_only", "singleplayer_only")
	} else {
		values = append(values, "server_only", "dedicated_server_only")
	}
	facets := make([]string, 0, len(values))
	for _, value := range values {
		facets = append(facets, "environment:"+value)
	}
	return facets
}

func MatchesEnvironment(environment, side string) bool {
	if side != "client" && side != "server" {
		return false
	}
	return slices.Contains(environmentFacets(side), "environment:"+environment)
}
func (c *Client) categoriesModrinth(ctx context.Context, kind Kind) ([]Category, error) {
	var raw []struct {
		Name        string `json:"name"`
		ProjectType string `json:"project_type"`
		Header      string `json:"header"`
		Icon        string `json:"icon"`
	}
	if err := c.get(ctx, Modrinth, "/tag/category", nil, &raw); err != nil {
		return nil, err
	}
	if len(raw) > 2000 {
		return nil, errors.New("市场类别数量超出限制")
	}
	var result []Category
	for _, entry := range raw {
		if entry.ProjectType == projectType(kind) || (kind == DataPack && entry.ProjectType == "mod") {
			if !slices.Contains(knownLoaders, entry.Name) {
				icon := entry.Icon
				if len(icon) > 32<<10 {
					icon = ""
				}
				result = append(result, Category{ID: entry.Name, Name: entry.Name, Header: entry.Header, IconSVG: icon})
			}
		}
	}
	return result, nil
}

type mrVersion struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"project_id"`
	Name         string    `json:"name"`
	Number       string    `json:"version_number"`
	Type         string    `json:"version_type"`
	Changelog    string    `json:"changelog"`
	Environment  string    `json:"environment"`
	GameVersions []string  `json:"game_versions"`
	Loaders      []string  `json:"loaders"`
	Published    time.Time `json:"date_published"`
	Downloads    int64     `json:"downloads"`
	Files        []struct {
		URL      string `json:"url"`
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
		Primary  bool   `json:"primary"`
		Hashes   struct {
			SHA1 string `json:"sha1"`
		} `json:"hashes"`
	} `json:"files"`
	Dependencies []struct {
		ProjectID string `json:"project_id"`
		VersionID string `json:"version_id"`
		Type      string `json:"dependency_type"`
	} `json:"dependencies"`
}

func (raw mrVersion) version() Version {
	changelog := raw.Changelog
	if changelog != "" {
		changelog = NormalizeMarkdown(changelog)
	}
	v := Version{Source: Modrinth, ID: raw.ID, ProjectID: raw.ProjectID, Name: raw.Name, Number: raw.Number, Type: raw.Type, Changelog: changelog, GameVersions: raw.GameVersions, Loaders: raw.Loaders, Published: raw.Published, Downloads: raw.Downloads, Environment: raw.Environment}
	for _, f := range raw.Files {
		v.Files = append(v.Files, File{URL: f.URL, Filename: f.Filename, SHA1: f.Hashes.SHA1, Size: f.Size, Primary: f.Primary})
	}
	for _, d := range raw.Dependencies {
		v.Dependencies = append(v.Dependencies, Dependency{ProjectID: d.ProjectID, VersionID: d.VersionID, Required: d.Type == "required", Incompatible: d.Type == "incompatible"})
	}
	return v
}
func (c *Client) versionsModrinth(ctx context.Context, id, game, loader string) ([]Version, error) {
	values := url.Values{"include_changelog": {"false"}}
	if game != "" {
		data, _ := json.Marshal([]string{game})
		values.Set("game_versions", string(data))
	}
	if loader != "" {
		data, _ := json.Marshal([]string{loader})
		values.Set("loaders", string(data))
	}
	var raw []mrVersion
	if err := c.get(ctx, Modrinth, "/project/"+id+"/version", values, &raw); err != nil {
		return nil, err
	}
	result := make([]Version, 0, len(raw))
	for _, entry := range raw {
		v := entry.version()
		if v.ProjectID == id && ValidID(v.ID) {
			result = append(result, v)
		}
	}
	slices.SortStableFunc(result, func(a, b Version) int { return b.Published.Compare(a.Published) })
	return result, nil
}
