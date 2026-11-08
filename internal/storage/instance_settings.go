package storage

import (
	"bytes"
	"context"
	"errors"
	"os"

	"mcl/internal/app"
	"mcl/internal/core/instance"
)

// UpdatePreferences replaces only the user-editable fields of an instance. It
// refuses the write when the stored name, Java path, icon style or icon no
// longer match expected, so a newer edit is reported instead of overwritten.
// Every other field, including the version, loader, pack identity, creation time
// and legacy heap value, is preserved.
func (s *InstanceStore) UpdatePreferences(ctx context.Context, id string, expected, desired app.InstancePreferences) (instance.Instance, error) {
	if !instance.ValidID(id) {
		return instance.Instance{}, errors.New("invalid instance ID")
	}
	if err := ctx.Err(); err != nil {
		return instance.Instance{}, err
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return instance.Instance{}, err
	}
	defer root.Close()
	value, err := readInstance(ctx, root, id)
	if err != nil {
		return instance.Instance{}, err
	}
	if !samePreferences(app.PreferencesForInstance(value), expected) {
		return instance.Instance{}, app.ErrInstanceSettingsChanged
	}
	value.Name = desired.Name
	value.JavaPath = desired.JavaPath
	value.IconStyle = desired.IconStyle
	value.Icon = bytes.Clone(desired.Icon)
	if err := value.Validate(); err != nil {
		return instance.Instance{}, err
	}
	// The caller holds the instance lease throughout this compare-and-write.
	// Check cancellation again before the atomic replacement.
	if err := ctx.Err(); err != nil {
		return instance.Instance{}, err
	}
	if err := writeInstance(ctx, root, value); err != nil {
		return instance.Instance{}, err
	}
	return value, nil
}

func samePreferences(current, expected app.InstancePreferences) bool {
	return current.Name == expected.Name &&
		current.JavaPath == expected.JavaPath &&
		current.IconStyle == expected.IconStyle &&
		bytes.Equal(current.Icon, expected.Icon)
}
