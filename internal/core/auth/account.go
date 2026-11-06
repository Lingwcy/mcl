package auth

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"image"
	"image/draw"
	"image/png"
	"regexp"
	"strings"
)

type Kind string

const (
	Microsoft Kind = "microsoft"
	Offline   Kind = "offline"
)

var playerName = regexp.MustCompile(`^[A-Za-z0-9_]{3,16}$`)

// Profile contains public account metadata. Avatar, when present, is a
// normalized 8×8 PNG head, not a reference to a local or remote skin file.
type Profile struct {
	Kind Kind   `json:"kind"`
	Name string `json:"name"`
	UUID string `json:"uuid"`
	Head []byte `json:"head,omitempty"`
}

func (p Profile) ID() string { return string(p.Kind) + ":" + p.UUID }

func (p Profile) Validate() error {
	_, err := hex.DecodeString(p.UUID)
	if !playerName.MatchString(p.Name) || len(p.UUID) != 32 || err != nil || p.UUID != strings.ToLower(p.UUID) {
		return errors.New("玩家名称或 UUID 无效")
	}
	switch p.Kind {
	case Microsoft:
	case Offline:
		if p.UUID != offlineUUID(p.Name) {
			return errors.New("离线玩家 UUID 无效")
		}
	default:
		return errors.New("未知账户类型")
	}
	if len(p.Head) > 1024 {
		return errors.New("玩家头像过大")
	}
	if len(p.Head) > 0 {
		cfg, err := png.DecodeConfig(bytes.NewReader(p.Head))
		if err != nil || cfg.Width != 8 || cfg.Height != 8 {
			return errors.New("玩家头像格式无效")
		}
	}
	return nil
}

func OfflineProfile(name string) (Profile, error) {
	name = strings.TrimSpace(name)
	if !playerName.MatchString(name) {
		return Profile{}, errors.New("离线 ID 需要 3–16 个字母、数字或下划线")
	}
	return Profile{Kind: Offline, Name: name, UUID: offlineUUID(name)}, nil
}

// Java Edition's name-based OfflinePlayer UUID. This MD5 is only an
// interoperability identifier and makes no authenticity claim.
func offlineUUID(name string) string {
	hash := md5.Sum([]byte("OfflinePlayer:" + name))
	hash[6] = (hash[6] & 0x0f) | 0x30
	hash[8] = (hash[8] & 0x3f) | 0x80
	return hex.EncodeToString(hash[:])
}

const MaxSkinBytes = 2 << 20

// SkinHead extracts the face and outer face layer of a Java skin.
func SkinHead(data []byte) ([]byte, error) {
	if len(data) > MaxSkinBytes {
		return nil, errors.New("皮肤 PNG 超过大小限制")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 64 || cfg.Width > 1024 || cfg.Width%64 != 0 || (cfg.Height != cfg.Width && cfg.Height*2 != cfg.Width) {
		return nil, errors.New("请选择标准 Minecraft Java 版 PNG 皮肤")
	}
	skin, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("无法读取皮肤 PNG")
	}
	scale := cfg.Width / 64
	face := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	hat := image.NewNRGBA(face.Bounds())
	for y := range 8 {
		for x := range 8 {
			face.Set(x, y, skin.At((8+x)*scale, (8+y)*scale))
			hat.Set(x, y, skin.At((40+x)*scale, (8+y)*scale))
		}
	}
	// Old opaque skins may contain a solid unused outer face in their atlas.
	// Modern skins always retain their authored outer layer.
	if cfg.Height == cfg.Width || !hat.Opaque() {
		draw.Draw(face, face.Bounds(), hat, image.Point{}, draw.Over)
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, face); err != nil {
		return nil, errors.New("无法生成玩家头像")
	}
	return encoded.Bytes(), nil
}
