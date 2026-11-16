// Localized resource metadata database and query/translation helpers ported from SJMCL:
// - Offline MCMod mapping database: internal/core/market/mod_data.csv.gz (from SJMCL src-tauri/assets/db/mod_data.csv)
// - Online description translation service: https://mod.mcimirror.top/translate/{modrinth,curseforge}/{id}

package market

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"mcl/internal/core/meta"
)

//go:embed mod_data.csv.gz
var modDataCSVGz []byte

// MCModRecord represents one localized project record from the embedded MCMod database.
type MCModRecord struct {
	MCModID        uint32
	CurseForgeSlug string
	ModrinthSlug   string
	Name           string
	Subname        string
	Abbr           string
}

type modDatabase struct {
	mods            []MCModRecord
	modrinthToMod   map[string]uint32
	curseforgeToMod map[string]uint32
}

var (
	modDBOnce sync.Once
	modDBInst *modDatabase
)

func getModDB() *modDatabase {
	modDBOnce.Do(func() {
		modDBInst = loadModDB()
	})
	return modDBInst
}

func loadModDB() *modDatabase {
	db := &modDatabase{
		modrinthToMod:   make(map[string]uint32, 16384),
		curseforgeToMod: make(map[string]uint32, 24576),
	}
	if len(modDataCSVGz) == 0 {
		return db
	}
	gz, err := gzip.NewReader(bytes.NewReader(modDataCSVGz))
	if err != nil {
		return db
	}
	defer gz.Close()

	reader := csv.NewReader(gz)
	headers, err := reader.Read()
	if err != nil || len(headers) == 0 {
		return db
	}
	headers[0] = strings.TrimPrefix(headers[0], "\uFEFF")

	colIdx := make(map[string]int, len(headers))
	for i, h := range headers {
		colIdx[strings.TrimSpace(h)] = i
	}
	idIdx, okID := colIdx["mcmod_id"]
	cfIdx, okCF := colIdx["curseforge_slug"]
	mrIdx, okMR := colIdx["modrinth_slug"]
	nameIdx, okName := colIdx["name"]
	subIdx, okSub := colIdx["subname"]
	abbrIdx, okAbbr := colIdx["abbr"]
	if !okID || !okCF || !okMR || !okName || !okSub || !okAbbr {
		return db
	}

	db.mods = make([]MCModRecord, 0, 32000)
	for {
		rec, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || len(rec) <= max(idIdx, cfIdx, mrIdx, nameIdx, subIdx, abbrIdx) {
			continue
		}
		id64, err := strconv.ParseUint(strings.TrimSpace(rec[idIdx]), 10, 32)
		if err != nil || id64 == 0 {
			continue
		}
		mcmodID := uint32(id64)
		row := MCModRecord{
			MCModID:        mcmodID,
			CurseForgeSlug: strings.TrimSpace(rec[cfIdx]),
			ModrinthSlug:   strings.TrimSpace(rec[mrIdx]),
			Name:           strings.TrimSpace(rec[nameIdx]),
			Subname:        strings.TrimSpace(rec[subIdx]),
			Abbr:           strings.TrimSpace(rec[abbrIdx]),
		}
		db.mods = append(db.mods, row)
		idx := uint32(len(db.mods)) // 1-based slice index for fast direct lookup
		if row.CurseForgeSlug != "" {
			db.curseforgeToMod[row.CurseForgeSlug] = idx
			db.curseforgeToMod[strings.ToLower(row.CurseForgeSlug)] = idx
		}
		if row.ModrinthSlug != "" {
			db.modrinthToMod[row.ModrinthSlug] = idx
			db.modrinthToMod[strings.ToLower(row.ModrinthSlug)] = idx
		}
	}
	return db
}

func containsChinese(s string) bool {
	for _, r := range s {
		if (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0x3400 && r <= 0x4DBF) || (r >= 0xF900 && r <= 0xFAFF) {
			return true
		}
	}
	return false
}

// GetTranslatedName looks up the Chinese name of a mod from the embedded MCMod database.
func GetTranslatedName(slug string, source Source) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return ""
	}
	db := getModDB()
	var table map[string]uint32
	switch source {
	case Modrinth:
		table = db.modrinthToMod
	case CurseForge:
		table = db.curseforgeToMod
	default:
		return ""
	}
	idx := table[slug]
	if idx == 0 {
		idx = table[strings.ToLower(slug)]
	}
	if idx == 0 || int(idx) > len(db.mods) {
		return ""
	}
	name := db.mods[idx-1].Name
	if !containsChinese(name) {
		return ""
	}
	return name
}

var searchStopWords = map[string]bool{
	"a": true, "of": true, "the": true, "for": true,
	"mod": true, "with": true, "and": true, "ftb": true,
}

func cleanKeyword(word string) (string, bool) {
	cleaned := strings.TrimLeft(word, "{[(\"'")
	cleaned = strings.TrimRight(cleaned, "}])\"'")
	cleaned = strings.Trim(cleaned, "-_")
	cleaned = strings.ToLower(cleaned)
	if cleaned == "" || searchStopWords[cleaned] {
		return "", false
	}
	if _, err := strconv.ParseFloat(cleaned, 64); err == nil {
		return "", false
	}
	return cleaned, true
}

func extractSearchWords(text string) []string {
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	words := make([]string, 0, len(parts))
	for _, p := range parts {
		if w, ok := cleanKeyword(p); ok {
			words = append(words, w)
		}
	}
	return words
}

func calculateSimilarity(source, query string) float64 {
	sourceClean := strings.ReplaceAll(strings.ToLower(source), " ", "")
	queryClean := strings.ReplaceAll(strings.ToLower(query), " ", "")
	sourceChars := []rune(sourceClean)
	queryChars := []rune(queryClean)
	if len(queryChars) == 0 || len(sourceChars) == 0 {
		return 0
	}

	sourceLength := len(sourceClean)
	queryLength := len(queryClean)
	qp := 0
	lenSum := 0.0

	for qp < len(queryChars) {
		sp := 0
		lenMax := 0
		spMax := 0
		for sp < len(sourceChars) {
			l := 0
			for qp+l < len(queryChars) && sp+l < len(sourceChars) && sourceChars[sp+l] == queryChars[qp+l] {
				l++
			}
			if l > lenMax {
				lenMax = l
				spMax = sp
			}
			step := l
			if step < 1 {
				step = 1
			}
			sp += step
		}
		if lenMax > 0 {
			sourceChars = append(sourceChars[:spMax], sourceChars[spMax+lenMax:]...)
			incWeight := math.Max(math.Pow(1.4, float64(3+lenMax))-3.6, 0)
			dist := qp - spMax
			if dist < 0 {
				dist = -dist
			}
			posBonus := 1.0 + 0.3*math.Max(float64(3-dist), 0)
			lenSum += incWeight * posBonus
		}
		if lenMax > 0 {
			qp += lenMax
		} else {
			qp++
		}
	}

	baseScore := lenSum / float64(queryLength)
	lengthFactor := 3.0 / math.Sqrt(float64(sourceLength)+15.0)
	shortBonus := 1.0
	if queryLength <= 2 {
		shortBonus = float64(3 - queryLength)
	}
	return baseScore * lengthFactor * shortBonus
}

func isAbsoluteMatch(source, query string) bool {
	sourceClean := strings.ToLower(strings.ReplaceAll(source, " ", ""))
	for _, part := range strings.Fields(query) {
		if !strings.Contains(sourceClean, strings.ToLower(part)) {
			return false
		}
	}
	return true
}

func (db *modDatabase) getModsByChinese(query string, maxResults int) []MCModRecord {
	processed := strings.Join(strings.Fields(query), " ")
	if processed == "" || len(db.mods) == 0 {
		return nil
	}

	type searchEntry struct {
		record        MCModRecord
		similarity    float64
		absoluteMatch bool
	}
	entries := make([]searchEntry, 0, 128)
	queryCharCount := 0
	for _, r := range processed {
		if !unicode.IsSpace(r) {
			queryCharCount++
		}
	}
	minSimilarity := 0.05
	switch {
	case queryCharCount == 1:
		minSimilarity = 0.15
	case queryCharCount == 2:
		minSimilarity = 0.12
	case queryCharCount >= 3 && queryCharCount <= 4:
		minSimilarity = 0.08
	}

	for _, rec := range db.mods {
		abs := isAbsoluteMatch(rec.Name, processed)
		sim := calculateSimilarity(rec.Name, processed)
		if abs || sim >= minSimilarity {
			entries = append(entries, searchEntry{
				record:        rec,
				similarity:    sim,
				absoluteMatch: abs,
			})
		}
	}
	slices.SortFunc(entries, func(a, b searchEntry) int {
		if a.absoluteMatch != b.absoluteMatch {
			if a.absoluteMatch {
				return -1
			}
			return 1
		}
		return cmp.Compare(b.similarity, a.similarity)
	})

	results := make([]MCModRecord, 0, maxResults*2)
	blurCount := 0
	for _, entry := range entries {
		if entry.absoluteMatch {
			results = append(results, entry.record)
		} else if blurCount < maxResults {
			results = append(results, entry.record)
			blurCount++
		}
		if len(results) >= maxResults*2 {
			break
		}
	}
	return results
}

// RewriteSearchQuery converts Chinese mod search terms into English search
// keywords using the embedded MCMod database (porting SJMCL handle_localized_search_query).
func RewriteSearchQuery(query string) string {
	normalized := strings.Join(strings.Fields(query), " ")
	if normalized == "" || !containsChinese(normalized) {
		return normalized
	}
	db := getModDB()
	searchResults := db.getModsByChinese(normalized, 3)
	if len(searchResults) == 0 {
		return normalized
	}

	var bestRecord *MCModRecord
	bestSim := -1.0
	bestAbs := false
	compactQuery := strings.ReplaceAll(normalized, " ", "")
	for i := range searchResults {
		rec := &searchResults[i]
		if rec.Subname == "" && rec.CurseForgeSlug == "" && rec.ModrinthSlug == "" {
			continue
		}
		sim := calculateSimilarity(rec.Name, normalized)
		abs := strings.ReplaceAll(rec.Name, " ", "") == compactQuery
		if bestRecord == nil || sim > bestSim {
			bestRecord = rec
			bestSim = sim
			bestAbs = abs
		}
	}
	if bestRecord != nil && (bestAbs || bestSim >= 0.9) {
		exact := bestRecord.Subname
		if exact == "" {
			exact = bestRecord.CurseForgeSlug
		}
		if exact == "" {
			exact = bestRecord.ModrinthSlug
		}
		if exact != "" {
			runes := []rune(exact)
			if len(runes) > 24 {
				runes = runes[:24]
			}
			return string(runes)
		}
	}

	keywordCount := make(map[string]int)
	totalMods := len(searchResults)
	for _, rec := range searchResults {
		seen := make(map[string]bool)
		for _, text := range []string{rec.Subname, rec.CurseForgeSlug, rec.ModrinthSlug} {
			if text == "" {
				continue
			}
			for _, kw := range extractSearchWords(text) {
				seen[kw] = true
			}
		}
		for kw := range seen {
			keywordCount[kw]++
		}
	}
	if len(keywordCount) == 0 {
		return normalized
	}

	type kwScore struct {
		kw    string
		score float64
	}
	scores := make([]kwScore, 0, len(keywordCount))
	for kw, cnt := range keywordCount {
		freq := float64(cnt) / float64(totalMods)
		bonus := math.Min(float64(len(kw))/10.0, 1.0) + 1.0
		scores = append(scores, kwScore{kw: kw, score: freq * bonus})
	}
	slices.SortFunc(scores, func(a, b kwScore) int {
		if d := cmp.Compare(b.score, a.score); d != 0 {
			return d
		}
		return cmp.Compare(a.kw, b.kw)
	})

	minThreshold := int(math.Max(float64(totalMods)*0.4, 1.0))
	minFreqRatio := float64(minThreshold) / float64(totalMods)
	primary := scores[0]
	finalKeywords := []string{primary.kw}
	for _, item := range scores[1:] {
		if len(finalKeywords) >= 2 || len(primary.kw) >= 16 {
			break
		}
		freqRatio := float64(keywordCount[item.kw]) / float64(totalMods)
		if freqRatio < minFreqRatio {
			continue
		}
		scoreRatio := item.score / math.Max(primary.score, 1e-6)
		if scoreRatio >= 0.5 {
			finalKeywords = append(finalKeywords, item.kw)
		}
	}
	if len(finalKeywords) == 0 {
		return normalized
	}
	return strings.Join(finalKeywords, " ")
}

const translationCacheTTLSeconds int64 = 30 * 24 * 3600

type translationCacheEntry struct {
	TranslatedName        string `json:"translated_name,omitempty"`
	TranslatedDescription string `json:"translated_description,omitempty"`
	DescriptionTranslated bool   `json:"description_translated"`
	Timestamp             int64  `json:"timestamp"`
}

func (e translationCacheEntry) expired(now int64) bool {
	return now > e.Timestamp+translationCacheTTLSeconds
}

var translationCache struct {
	mu      sync.Mutex
	path    string
	loaded  bool
	entries map[string]translationCacheEntry
}

// SetTranslationCachePath sets the persistent JSON cache path for online
// resource description translations (e.g. <root>/resource_description_translations.json).
func SetTranslationCachePath(cachePath string) {
	translationCache.mu.Lock()
	defer translationCache.mu.Unlock()
	if translationCache.path == cachePath {
		return
	}
	translationCache.path = cachePath
	translationCache.loaded = false
	translationCache.entries = nil
}

func ensureTranslationCacheLocked() {
	if translationCache.loaded {
		return
	}
	translationCache.loaded = true
	if translationCache.entries == nil {
		translationCache.entries = make(map[string]translationCacheEntry)
	}
	if translationCache.path == "" {
		return
	}
	info, err := os.Lstat(translationCache.path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 16<<20 {
		return
	}
	data, err := os.ReadFile(translationCache.path)
	if err != nil {
		return
	}
	var parsed map[string]translationCacheEntry
	if json.Unmarshal(data, &parsed) == nil && parsed != nil {
		translationCache.entries = parsed
	}
}

func lookupTranslationCache(key string) (translationCacheEntry, bool) {
	translationCache.mu.Lock()
	defer translationCache.mu.Unlock()
	ensureTranslationCacheLocked()
	entry, ok := translationCache.entries[key]
	if !ok || entry.expired(time.Now().Unix()) {
		return translationCacheEntry{}, false
	}
	return entry, true
}

func saveTranslationCacheEntries(updates map[string]translationCacheEntry) {
	if len(updates) == 0 {
		return
	}
	translationCache.mu.Lock()
	ensureTranslationCacheLocked()
	for k, v := range updates {
		translationCache.entries[k] = v
	}
	cachePath := translationCache.path
	var payload []byte
	if cachePath != "" {
		payload, _ = json.Marshal(translationCache.entries)
	}
	translationCache.mu.Unlock()

	if cachePath == "" || len(payload) == 0 {
		return
	}
	dir := filepath.Dir(cachePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return
	}
	defer root.Close()
	tmp := ".mcl-trans-" + rand.Text() + ".tmp"
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return
	}
	_, writeErr := f.Write(payload)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		_ = root.Remove(tmp)
		return
	}
	if err := root.Rename(tmp, filepath.Base(cachePath)); err != nil {
		_ = root.Remove(tmp)
	}
}

// TranslateDescription fetches the Simplified Chinese description translation
// for a Modrinth or CurseForge project from MCIM. Network or HTTP errors are
// swallowed and return ("", nil) so market browsing is never blocked.
func (c *Client) TranslateDescription(ctx context.Context, source Source, id string) (string, error) {
	if c == nil || c.TranslateURL == "" || !ValidID(id) {
		return "", nil
	}
	var endpoint string
	switch source {
	case Modrinth:
		endpoint = "/translate/modrinth/" + url.PathEscape(id)
	case CurseForge:
		endpoint = "/translate/curseforge/" + url.PathEscape(id)
	default:
		return "", nil
	}
	base := strings.TrimRight(c.TranslateURL, "/")
	u, err := url.Parse(base + endpoint)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" {
		return "", nil
	}
	reqCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", nil
	}
	req.Header.Set("User-Agent", "mcl/0.1.0 (native Minecraft launcher)")
	req.Header.Set("Accept", "application/json")

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", nil
	}
	var decoded struct {
		Translated string `json:"translated"`
	}
	if json.Unmarshal(data, &decoded) != nil {
		return "", nil
	}
	return strings.TrimSpace(decoded.Translated), nil
}

// EnhanceProjects populates TranslatedName from the embedded MCMod database for mods
// and, when translateDesc is true, resolves TranslatedDescription concurrently using
// the translation cache and MCIM translation API.
func (c *Client) EnhanceProjects(ctx context.Context, projects []Project, translateDesc bool) {
	if len(projects) == 0 {
		return
	}
	type workItem struct {
		idx int
		src Source
		id  string
		key string
		tn  string
	}
	var misses []workItem
	for i := range projects {
		p := &projects[i]
		if p.Kind == Mod && p.TranslatedName == "" {
			if name := GetTranslatedName(p.Slug, p.Source); name != "" {
				p.TranslatedName = name
			}
		}
		if !translateDesc || c == nil || c.TranslateURL == "" || !ValidID(p.ID) {
			continue
		}
		key := fmt.Sprintf("%s:%s", p.Source, p.ID)
		if cached, ok := lookupTranslationCache(key); ok {
			if p.TranslatedName == "" && cached.TranslatedName != "" {
				p.TranslatedName = cached.TranslatedName
			}
			if cached.TranslatedDescription != "" || cached.DescriptionTranslated {
				p.TranslatedDescription = cached.TranslatedDescription
				continue
			}
		}
		misses = append(misses, workItem{idx: i, src: p.Source, id: p.ID, key: key, tn: p.TranslatedName})
	}
	if len(misses) == 0 || ctx.Err() != nil {
		return
	}

	var mu sync.Mutex
	updates := make(map[string]translationCacheEntry, len(misses))
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	now := time.Now().Unix()
	for _, item := range misses {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(w workItem) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			desc, _ := c.TranslateDescription(ctx, w.src, w.id)
			if ctx.Err() != nil && desc == "" {
				return
			}
			mu.Lock()
			if desc != "" {
				projects[w.idx].TranslatedDescription = desc
			}
			updates[w.key] = translationCacheEntry{
				TranslatedName:        w.tn,
				TranslatedDescription: desc,
				DescriptionTranslated: desc != "",
				Timestamp:             now,
			}
			mu.Unlock()
		}(item)
	}
	wg.Wait()
	if ctx.Err() == nil && len(updates) > 0 {
		saveTranslationCacheEntries(updates)
	}
}

// TranslatedProject formats the display title and description for a project
// when resource translation is enabled.
func TranslatedProject(p Project) (title, description string) {
	title = p.Title
	if p.TranslatedName != "" && p.TranslatedName != p.Title {
		title = p.TranslatedName + " | " + p.Title
	}
	description = p.Description
	if p.TranslatedDescription != "" {
		description = p.TranslatedDescription
	}
	return title, description
}

// PrefixedFilename prepends "[<TranslatedName>] " to filename when translated
// metadata is available and safe for managed installation paths.
func PrefixedFilename(filename string, p Project) string {
	if !SafeFilename(filename) {
		return filename
	}
	if strings.HasPrefix(filename, "[") && strings.Contains(filename, "] ") {
		return filename
	}
	name := strings.TrimSpace(p.TranslatedName)
	if name == "" && p.Kind == Mod && p.Slug != "" {
		name = GetTranslatedName(p.Slug, p.Source)
	}
	if name == "" {
		return filename
	}
	candidate := "[" + name + "] " + filename
	if SafeFilename(candidate) && meta.ValidPath(path.Join("mods", candidate)) {
		return candidate
	}
	return filename
}

// HasTranslatedPrefix reports whether baseName was derived from originalFilename
// by prepending a valid "[<tag>] " prefix.
func HasTranslatedPrefix(baseName, originalFilename string) bool {
	if !SafeFilename(baseName) || !meta.ValidID(baseName) || !strings.HasPrefix(baseName, "[") {
		return false
	}
	closeIdx := strings.Index(baseName, "] ")
	if closeIdx <= 1 || closeIdx > 60 {
		return false
	}
	prefix := baseName[1:closeIdx]
	if strings.ContainsAny(prefix, "[]") || strings.TrimSpace(prefix) == "" {
		return false
	}
	return baseName[closeIdx+2:] == originalFilename
}
