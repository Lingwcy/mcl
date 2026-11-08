// Package instance defines isolated game installations and their user settings.
package instance

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"mcl/internal/core/meta"
)

type Instance struct {
	GameDirectoryID string             `json:"game_directory_id,omitempty"`
	ID              string             `json:"id"`
	Name            string             `json:"name"`
	Description     string             `json:"description,omitempty"`
	Version         meta.ManifestEntry `json:"version"`
	RequiredJava    int                `json:"required_java"`
	JavaPath        string             `json:"java_path"`
	MaxMemoryMiB    int                `json:"max_memory_mib"`
	CreatedAt       time.Time          `json:"created_at"`
	Pack            *Pack              `json:"pack,omitempty"`
	Loader          *Loader            `json:"loader,omitempty"`
	Icon            []byte             `json:"icon,omitempty"`
	IconStyle       string             `json:"icon_style,omitempty"`
}

type Loader struct {
	Type    string `json:"type"`
	Version string `json:"version"`
}
type Pack struct {
	Name        string   `json:"name,omitempty"`
	Version     string   `json:"version,omitempty"`
	Author      string   `json:"author,omitempty"`
	Source      string   `json:"source"`
	ProjectID   string   `json:"project_id"`
	VersionID   string   `json:"version_id"`
	ArchiveSHA1 string   `json:"archive_sha1"`
	Local       bool     `json:"local,omitempty"`
	Loaders     []Loader `json:"loaders,omitempty"`
}

// RequiredLoader returns the explicit runtime or one pinned by a pack. It is
// not inferred from the installed mod files or from a market search filter.
func (i Instance) RequiredLoader() (Loader, bool) {
	if i.Loader != nil {
		return *i.Loader, true
	}
	if i.Pack != nil && len(i.Pack.Loaders) == 1 {
		return i.Pack.Loaders[0], true
	}
	return Loader{}, false
}

func NewID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		// Go's crypto/rand never returns an error on supported systems. Keep the
		// failure fatal rather than produce a predictable installation identifier.
		panic(err)
	}
	return hex.EncodeToString(bytes[:])
}

func ValidID(id string) bool {
	if len(id) != 32 || strings.ToLower(id) != id {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func ValidateName(name string) error {
	if name != strings.TrimSpace(name) || name == "" || utf8.RuneCountInString(name) > 80 || !utf8.ValidString(name) || strings.IndexFunc(name, unicode.IsControl) != -1 {
		return errors.New("实例名称应为 1 到 80 个字符，且不能包含控制字符或首尾空白")
	}
	return nil
}

func (i Instance) Validate() error {
	if (i.GameDirectoryID != "" && !ValidID(i.GameDirectoryID)) || !ValidID(i.ID) || i.RequiredJava <= 0 || i.CreatedAt.IsZero() {
		return errors.New("invalid instance ID, Java requirement or creation time")
	}
	if err := ValidateDescription(i.Description); err != nil {
		return err
	}
	if err := ValidateName(i.Name); err != nil {
		return err
	}
	if len(i.Icon) > 128<<10 || !ValidIconStyle(i.IconStyle) {
		return errors.New("invalid instance icon")
	}
	if len(i.Icon) > 0 && (len(i.Icon) < 33 || string(i.Icon[:8]) != "\x89PNG\r\n\x1a\n" || string(i.Icon[12:16]) != "IHDR" || binary.BigEndian.Uint32(i.Icon[16:20]) < 1 || binary.BigEndian.Uint32(i.Icon[16:20]) > 80 || binary.BigEndian.Uint32(i.Icon[20:24]) < 1 || binary.BigEndian.Uint32(i.Icon[20:24]) > 80) {
		return errors.New("invalid normalized instance icon")
	}
	if i.Loader != nil {
		if i.Pack != nil || !ValidLoader(*i.Loader) {
			return errors.New("invalid or conflicting instance loader")
		}
	}
	if (i.MaxMemoryMiB != 0 && (i.MaxMemoryMiB < 512 || i.MaxMemoryMiB > 65536)) || strings.ContainsRune(i.JavaPath, '\x00') || (i.JavaPath != "" && !filepath.IsAbs(i.JavaPath)) {
		return errors.New("invalid instance Java path or memory setting")
	}
	if i.Pack != nil {
		p := i.Pack
		if len(p.Name) > 1024 || len(p.Version) > 256 || len(p.Author) > 1024 || !utf8.ValidString(p.Name+p.Version+p.Author) || strings.IndexFunc(p.Name+p.Version+p.Author, unicode.IsControl) >= 0 {
			return errors.New("invalid modpack metadata")
		}
		if (p.Source != "modrinth" && p.Source != "curseforge" && !(p.Source == "multimc" && p.Local)) || p.ProjectID == "" || p.VersionID == "" || !meta.ValidSHA1(p.ArchiveSHA1) || len(p.Loaders) > 1 {
			return errors.New("invalid modpack identity or runtime")
		}
		if p.Local && (p.ProjectID != "local-"+p.ArchiveSHA1 || p.VersionID != p.ArchiveSHA1) {
			return errors.New("invalid local modpack identity")
		}
		for _, loader := range p.Loaders {
			if loader.Type != "fabric" && loader.Type != "quilt" && loader.Type != "forge" && loader.Type != "neoforge" {
				return errors.New("unsupported modpack loader")
			}
			if loader.Version == "" || len(loader.Version) > 100 || strings.ContainsAny(loader.Version, "/\\\x00\r\n") {
				return errors.New("invalid modpack loader version")
			}
		}
	}
	return i.Version.Validate()
}

func ValidIconStyle(value string) bool {
	return value == "" || value == "grass" || value == "fabric" || value == "forge" || value == "neoforge" || value == "quilt" || value == "cat" || value == "leaf" || value == "spark" || value == "craft"
}

func ValidLoader(value Loader) bool {
	return (value.Type == "fabric" || value.Type == "quilt" || value.Type == "forge" || value.Type == "neoforge") &&
		meta.ValidID(value.Version) && !strings.ContainsAny(value.Version, " /\\:\r\n")
}

func ValidateDescription(value string) error {
	if !utf8.ValidString(value) || len(value) > 16<<10 || utf8.RuneCountInString(value) > 4096 || strings.IndexFunc(value, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\t' }) >= 0 {
		return errors.New("实例描述最多4096个字符，且不能含无效控制字符")
	}
	return nil
}
