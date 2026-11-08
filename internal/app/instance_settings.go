package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"path/filepath"
	"strings"

	"mcl/internal/core/instance"
	"mcl/internal/core/thumbnail"
)

// ErrInstanceSettingsChanged reports that stored settings no longer match the
// snapshot a caller edited. The caller reloads instead of overwriting a newer
// name, icon or Java preference written by another window or launcher process.
var ErrInstanceSettingsChanged = errors.New("实例设置已被其他进程修改，请重新读取后再保存")

// maxInstanceIconBytes mirrors the instance document limit for an embedded PNG.
const maxInstanceIconBytes = 128 << 10

// InstancePreferences holds only the fields a user edits from the instance
// settings view. The game version, loader and modpack identity stay fixed, and
// the legacy heap fields are neither read nor written here.
type InstancePreferences struct {
	Name      string `json:"name"`
	JavaPath  string `json:"java_path"`
	IconStyle string `json:"icon_style"`
	Icon      []byte `json:"icon,omitempty"`
}

// PreferencesForInstance copies the editable fields, including the bounded PNG
// icon, so a caller never shares the instance's backing array.
func PreferencesForInstance(value instance.Instance) InstancePreferences {
	return InstancePreferences{
		Name:      value.Name,
		JavaPath:  value.JavaPath,
		IconStyle: value.IconStyle,
		Icon:      bytes.Clone(value.Icon),
	}
}

// InstanceSettingsRepository reads an instance and replaces its editable
// fields only when they still match the caller's expected snapshot.
type InstanceSettingsRepository interface {
	Get(context.Context, string) (instance.Instance, error)
	UpdatePreferences(context.Context, string, InstancePreferences, InstancePreferences) (instance.Instance, error)
}

// InstanceSettingsService validates edits off the UI thread. Saving reserves the
// instance through the registry first, so a running game, another launcher
// process or an unfinished asset transaction is never written past.
type InstanceSettingsService struct {
	store    InstanceSettingsRepository
	registry SessionRegistry
}

func NewInstanceSettingsService(store InstanceSettingsRepository, registry SessionRegistry) *InstanceSettingsService {
	return &InstanceSettingsService{store: store, registry: registry}
}

// Load reads the authoritative document without reserving the instance. It only
// reads; callers use the result as the expected snapshot for Save.
func (s *InstanceSettingsService) Load(ctx context.Context, id string) (instance.Instance, error) {
	return s.store.Get(ctx, id)
}

// Save replaces the name, icon and Java preference. A new custom icon is normalized
// to a bounded 80×80 PNG copy in the background; the user's source path is never
// persisted. Java is validated as a path only, never executed or probed.
func (s *InstanceSettingsService) Save(ctx context.Context, id string, expected, desired InstancePreferences) (instance.Instance, error) {
	desired.Name = strings.TrimSpace(desired.Name)
	desired.JavaPath = strings.TrimSpace(desired.JavaPath)
	desired.IconStyle = strings.TrimSpace(desired.IconStyle)
	if err := instance.ValidateName(desired.Name); err != nil {
		return instance.Instance{}, err
	}
	if err := validateInstancePreferences(desired); err != nil {
		return instance.Instance{}, err
	}
	if bytes.Equal(desired.Icon, expected.Icon) {
		// An unrelated name/Java edit must not repeatedly resample or
		// re-encode the already stored custom artwork.
		desired.Icon = bytes.Clone(desired.Icon)
	} else {
		icon, err := normalizeInstanceIcon(desired.Icon)
		if err != nil {
			return instance.Instance{}, err
		}
		desired.Icon = icon
	}
	expected.Icon = bytes.Clone(expected.Icon)
	if err := ctx.Err(); err != nil {
		return instance.Instance{}, err
	}
	lease, err := s.registry.Reserve(ctx, id)
	if err != nil {
		return instance.Instance{}, err
	}
	defer lease.Close()
	return s.store.UpdatePreferences(ctx, id, expected, desired)
}

func validateInstancePreferences(preferences InstancePreferences) error {
	if strings.ContainsRune(preferences.JavaPath, '\x00') {
		return errors.New("Java 路径包含无效字符")
	}
	if preferences.JavaPath != "" && !filepath.IsAbs(preferences.JavaPath) {
		return errors.New("请输入 Java 可执行文件的绝对路径")
	}
	if !instance.ValidIconStyle(preferences.IconStyle) {
		return errors.New("实例图标选择无效")
	}
	return nil
}

// normalizeInstanceIcon decodes the bounded upload and re-encodes only the
// normalized PNG. The returned copy is independent of the caller's bytes.
func normalizeInstanceIcon(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	image, err := thumbnail.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("读取实例图标: %w", err)
	}
	var output bytes.Buffer
	if err := png.Encode(&output, image); err != nil {
		return nil, fmt.Errorf("保存实例图标: %w", err)
	}
	if output.Len() == 0 || output.Len() > maxInstanceIconBytes {
		return nil, errors.New("实例图标超出大小限制")
	}
	return output.Bytes(), nil
}
