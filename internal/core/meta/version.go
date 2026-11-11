// Package meta interprets Minecraft's version documents without UI dependencies.
package meta

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type Manifest struct {
	Latest struct {
		Release  string `json:"release"`
		Snapshot string `json:"snapshot"`
	} `json:"latest"`
	Versions []ManifestEntry `json:"versions"`
}

type ManifestEntry struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	URL         string    `json:"url"`
	SHA1        string    `json:"sha1"`
	ReleaseTime time.Time `json:"releaseTime"`
}

func ParseManifest(data []byte) (Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse version manifest: %w", err)
	}
	if len(manifest.Versions) == 0 {
		return Manifest{}, errors.New("version manifest is empty")
	}
	seen := make(map[string]bool, len(manifest.Versions))
	for _, entry := range manifest.Versions {
		if err := entry.Validate(); err != nil {
			return Manifest{}, err
		}
		if seen[entry.ID] {
			return Manifest{}, fmt.Errorf("duplicate version %q", entry.ID)
		}
		seen[entry.ID] = true
	}
	return manifest, nil
}

func (e ManifestEntry) Validate() error {
	if !ValidID(e.ID) || e.Type == "" || !ValidSHA1(e.SHA1) {
		return fmt.Errorf("invalid manifest entry %q", e.ID)
	}
	return ValidateURL(e.URL)
}

func (m Manifest) Find(id string) (ManifestEntry, bool) {
	for _, entry := range m.Versions {
		if entry.ID == id {
			return entry, true
		}
	}
	return ManifestEntry{}, false
}

func (m Manifest) Releases() []ManifestEntry {
	var entries []ManifestEntry
	for _, entry := range m.Versions {
		if entry.Type == "release" {
			entries = append(entries, entry)
		}
	}
	return entries
}

type Download struct {
	Path string `json:"path,omitempty"`
	URL  string `json:"url"`
	SHA1 string `json:"sha1"`
	Size int64  `json:"size"`
}

func (d Download) Validate() error {
	if err := d.ValidateIdentity(); err != nil {
		return err
	}
	return ValidateURL(d.URL)
}

func (d Download) ValidateIdentity() error {
	if !ValidSHA1(d.SHA1) || d.Size <= 0 {
		return errors.New("download has an invalid SHA-1 or size")
	}
	if d.Path != "" && !ValidPath(d.Path) {
		return fmt.Errorf("unsafe download path %q", d.Path)
	}
	return nil
}

type AssetIndexRef struct {
	Download
	ID        string `json:"id"`
	TotalSize int64  `json:"totalSize"`
}

type JavaVersion struct {
	Component string `json:"component"`
	Major     int    `json:"majorVersion"`
}

type Arguments struct {
	Game []Argument `json:"game"`
	JVM  []Argument `json:"jvm"`
}

type Library struct {
	LocalRoot string `json:"-"`
	Name      string `json:"name"`
	Downloads struct {
		Artifact    *Download           `json:"artifact"`
		Classifiers map[string]Download `json:"classifiers"`
	} `json:"downloads"`
	Natives map[string]string `json:"natives"`
	Extract struct {
		Exclude []string `json:"exclude"`
	} `json:"extract"`
	Rules []Rule `json:"rules"`
}

type Logging struct {
	Argument string `json:"argument"`
	Type     string `json:"type"`
	File     struct {
		Download
		ID string `json:"id"`
	} `json:"file"`
}

type Version struct {
	fields           map[string]json.RawMessage
	Sources          []ManifestEntry     `json:"-"`
	ClientVersion    string              `json:"-"`
	Jar              string              `json:"jar,omitempty"`
	Patches          []json.RawMessage   `json:"patches,omitempty"`
	LoaderProfile    string              `json:"-"` // Derived profile fingerprint; prevents layering a second loader.
	LibraryDirectory string              `json:"-"`
	RuntimeLookups   []Download          `json:"-"`
	LaunchName       string              `json:"-"`
	ID               string              `json:"id"`
	Type             string              `json:"type"`
	InheritsFrom     string              `json:"inheritsFrom"`
	MainClass        string              `json:"mainClass"`
	LegacyArguments  string              `json:"minecraftArguments"`
	Arguments        *Arguments          `json:"arguments"`
	Java             JavaVersion         `json:"javaVersion"`
	Downloads        map[string]Download `json:"downloads"`
	Libraries        []Library           `json:"libraries"`
	AssetIndex       AssetIndexRef       `json:"assetIndex"`
	Assets           string              `json:"assets"`
	MinimumLauncher  int                 `json:"minimumLauncherVersion"`
	ComplianceLevel  int                 `json:"complianceLevel"`
	Logging          struct {
		Client *Logging `json:"client"`
	} `json:"logging"`
}

func ParseVersion(data []byte) (Version, error) {
	if len(data) > maxMetadataSize {
		return Version{}, errors.New("版本文档超过16MiB")
	}
	var version Version
	if err := json.Unmarshal(data, &version); err != nil {
		return Version{}, fmt.Errorf("parse version: %w", err)
	}
	if err := json.Unmarshal(data, &version.fields); err != nil {
		return Version{}, err
	}
	partial := version.InheritsFrom != ""
	if !ValidID(version.ID) || (!partial && (version.Type == "" || version.MainClass == "")) || (partial && !ValidID(version.InheritsFrom)) {
		return Version{}, errors.New("version is missing its ID, type or main class, or has an unsafe parent ID")
	}
	return version, nil
}

// ValidateVanilla defines the currently implemented version format. Legacy and
// inherited documents are decoded but cannot silently enter the modern pipeline.
func (v Version) ValidateVanilla() error {
	if err := v.validateRuntimeLookups(); err != nil {
		return err
	}
	if v.LegacyArguments != "" && v.Arguments != nil {
		return errors.New("新旧参数模板冲突，必须先规范化")
	}
	if len(v.Patches) != 0 || v.Jar != "" {
		return errors.New("未知运行补丁或客户端引用尚未经过官方继承解析")
	}
	if strings.TrimSpace(v.MainClass) == "" {
		return errors.New("版本缺少有效主类")
	}
	if v.ClientVersion != "" && !ValidID(v.ClientVersion) {
		return errors.New("客户端 JAR 版本身份无效")
	}
	if (v.Type != "release" && v.Type != "snapshot") || v.InheritsFrom != "" || v.Arguments == nil {
		return errors.New("旧版参数与继承文档须先经官方身份核验及规范化，仅接受 release 或 snapshot")
	}
	if v.Java.Major <= 0 || len(v.Arguments.Game) == 0 || len(v.Arguments.JVM) == 0 {
		return errors.New("版本缺少 Java 要求或启动参数")
	}
	client, ok := v.Downloads["client"]
	if !ok {
		return errors.New("version has no client download")
	}
	if err := client.Validate(); err != nil {
		return fmt.Errorf("client: %w", err)
	}
	if !ValidID(v.AssetIndex.ID) {
		return errors.New("version has an invalid asset index ID")
	}
	if err := v.AssetIndex.Download.Validate(); err != nil {
		return fmt.Errorf("asset index: %w", err)
	}
	for _, library := range v.Libraries {
		if _, err := ParseCoordinate(library.Name); err != nil {
			return err
		}
		if err := ValidateRules(library.Rules); err != nil {
			return fmt.Errorf("library %s: %w", library.Name, err)
		}
		if library.Downloads.Artifact != nil {
			validate := library.Downloads.Artifact.Validate
			if library.LocalRoot != "" {
				if !ValidPath(library.LocalRoot) || library.LocalRoot != v.LibraryDirectory || library.Downloads.Artifact.URL != "" || len(library.Natives) > 0 || len(library.Downloads.Classifiers) > 0 {
					return fmt.Errorf("invalid prepared library %s", library.Name)
				}
				validate = library.Downloads.Artifact.ValidateIdentity
			}
			if err := validate(); err != nil {
				return fmt.Errorf("library %s: %w", library.Name, err)
			}
		}
		for _, native := range library.Downloads.Classifiers {
			if err := native.Validate(); err != nil {
				return err
			}
		}
	}
	for _, arguments := range [][]Argument{v.Arguments.Game, v.Arguments.JVM} {
		for _, argument := range arguments {
			if err := ValidateRules(argument.Rules); err != nil {
				return err
			}
		}
	}
	if logging := v.Logging.Client; logging != nil {
		if !ValidID(logging.File.ID) || !strings.Contains(logging.Argument, "${path}") {
			return errors.New("invalid logging configuration")
		}
		if err := logging.File.Download.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func ValidSHA1(value string) bool {
	if len(value) != 40 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// ValidID permits historical human-readable version IDs while excluding unsafe
// file-name components on every desktop platform, regardless of the host OS.
func ValidID(value string) bool {
	if value == "" || len(value) > 160 || !utf8.ValidString(value) || value == "." || value == ".." || strings.HasSuffix(value, ".") || strings.HasSuffix(value, " ") {
		return false
	}
	base, _, _ := strings.Cut(strings.ToUpper(value), ".")
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9') {
		return false
	}
	return !strings.ContainsAny(value, `/\:*?"<>|`) && strings.IndexFunc(value, unicode.IsControl) == -1
}

func ValidPath(value string) bool {
	if value == "" || strings.HasPrefix(value, "/") || path.Clean(value) != value {
		return false
	}
	for part := range strings.SplitSeq(value, "/") {
		if !ValidID(part) {
			return false
		}
	}
	return true
}

func ValidateURL(value string) error {
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return errors.New("invalid HTTP download URL")
	}
	return nil
}
