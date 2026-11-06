package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"mcl/internal/app"

	"github.com/egoist/mygo"
)

type AccountCredentials struct{ prefix string }

func NewAccountCredentials(dataRoot string) AccountCredentials {
	sum := sha256.Sum256([]byte(dataRoot))
	return AccountCredentials{prefix: "minecraft-refresh:" + hex.EncodeToString(sum[:]) + ":"}
}

func (s AccountCredentials) Get(id string) (string, error) {
	data, err := mygo.Secrets.Get(s.prefix + id)
	if errors.Is(err, mygo.ErrSecretNotFound) || (err == nil && len(data) == 0) {
		return "", app.ErrCredentialMissing
	}
	if err != nil {
		return "", errors.New("无法读取系统凭据，请解锁或启用系统凭据存储")
	}
	return string(data), nil
}

func (s AccountCredentials) Set(id, refreshToken string) error {
	if err := mygo.Secrets.Set(s.prefix+id, []byte(refreshToken)); err != nil {
		return errors.New("无法保存微软凭据，请检查系统凭据存储")
	}
	return nil
}

func (s AccountCredentials) Delete(id string) error {
	if err := mygo.Secrets.Delete(s.prefix + id); err != nil {
		return errors.New("无法清除系统凭据，请检查凭据存储后重试")
	}
	return nil
}
