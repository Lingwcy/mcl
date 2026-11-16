package server

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"image/png"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"mcl/internal/core/thumbnail"
)

const (
	tagEnd       byte = 0
	tagByte      byte = 1
	tagShort     byte = 2
	tagInt       byte = 3
	tagLong      byte = 4
	tagFloat     byte = 5
	tagDouble    byte = 6
	tagByteArray byte = 7
	tagString    byte = 8
	tagList      byte = 9
	tagCompound  byte = 10
	tagIntArray  byte = 11
	tagLongArray byte = 12

	maxNBTBytes      = 4 << 20
	maxNBTDepth      = 16
	maxNBTCollection = 4096
	maxNBTString     = 64 << 10
	maxIconBytes     = 128 << 10
)

// DiscoveredServer describes an entry parsed from an instance's servers.dat.
type DiscoveredServer struct {
	Name           string
	Address        string
	Key            string
	Icon           []byte
	AcceptTextures bool
	Hidden         bool
}

// ReadServersDat parses a bounded Minecraft servers.dat NBT stream.
func ReadServersDat(r io.Reader) ([]DiscoveredServer, error) {
	limited := &io.LimitedReader{R: r, N: maxNBTBytes + 1}
	br := bufio.NewReader(limited)
	header, err := br.Peek(2)
	if err != nil {
		if errors.Is(err, io.EOF) && len(header) == 0 {
			return nil, nil
		}
		if !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
			return nil, err
		}
	}
	var src io.Reader = br
	if len(header) >= 2 && header[0] == 0x1f && header[1] == 0x8b {
		gz, err := gzip.NewReader(br)
		if err != nil {
			return nil, fmt.Errorf("解压 servers.dat 失败: %w", err)
		}
		defer gz.Close()
		src = io.LimitReader(gz, maxNBTBytes+1)
	}
	nr := &nbtReader{r: src, limit: maxNBTBytes}
	rootType, err := nr.readByte()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, err
	}
	if rootType != tagCompound {
		return nil, errors.New("servers.dat 根节点不是 NBT Compound")
	}
	if _, err := nr.readString(); err != nil {
		return nil, err
	}
	var entries []DiscoveredServer
	seen := make(map[string]int)
	for {
		tagType, err := nr.readByte()
		if err != nil {
			return nil, err
		}
		if tagType == tagEnd {
			break
		}
		name, err := nr.readString()
		if err != nil {
			return nil, err
		}
		if name != "servers" || tagType != tagList {
			if err := nr.skipPayload(tagType, 1); err != nil {
				return nil, err
			}
			continue
		}
		list, err := nr.readServerList(1)
		if err != nil {
			return nil, err
		}
		for _, item := range list {
			if idx, ok := seen[item.Key]; ok {
				if entries[idx].Name == "" && item.Name != "" {
					entries[idx].Name = item.Name
				}
				if len(entries[idx].Icon) == 0 && len(item.Icon) > 0 {
					entries[idx].Icon = item.Icon
				}
				continue
			}
			seen[item.Key] = len(entries)
			entries = append(entries, item)
		}
	}
	return entries, nil
}

type nbtReader struct {
	r     io.Reader
	limit int64
	read  int64
}

func (n *nbtReader) Read(p []byte) (int, error) {
	if n.read+int64(len(p)) > n.limit {
		// Allow reading up to remaining bytes so short trailing reads still complete.
		remain := n.limit - n.read
		if remain <= 0 {
			return 0, errors.New("servers.dat 超出大小限制")
		}
		p = p[:remain]
	}
	k, err := n.r.Read(p)
	n.read += int64(k)
	return k, err
}

func (n *nbtReader) readFull(p []byte) error {
	if n.read+int64(len(p)) > n.limit {
		return errors.New("servers.dat 超出大小限制")
	}
	k, err := io.ReadFull(n.r, p)
	n.read += int64(k)
	return err
}

func (n *nbtReader) readByte() (byte, error) {
	var b [1]byte
	if err := n.readFull(b[:]); err != nil {
		return 0, err
	}
	return b[0], nil
}

func (n *nbtReader) readInt32() (int32, error) {
	var b [4]byte
	if err := n.readFull(b[:]); err != nil {
		return 0, err
	}
	return int32(binary.BigEndian.Uint32(b[:])), nil
}

func (n *nbtReader) readString() (string, error) {
	var b [2]byte
	if err := n.readFull(b[:]); err != nil {
		return "", err
	}
	length := int(binary.BigEndian.Uint16(b[:]))
	if length > maxNBTString {
		return "", errors.New("NBT 字符串超出大小限制")
	}
	if length == 0 {
		return "", nil
	}
	buf := make([]byte, length)
	if err := n.readFull(buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func (n *nbtReader) discard(count int64) error {
	if count < 0 {
		return errors.New("NBT 长度无效")
	}
	if n.read+count > n.limit {
		return errors.New("servers.dat 超出大小限制")
	}
	written, err := io.CopyN(io.Discard, n.r, count)
	n.read += written
	return err
}

func (n *nbtReader) skipPayload(tagType byte, depth int) error {
	if depth > maxNBTDepth {
		return errors.New("NBT 嵌套层级过深")
	}
	switch tagType {
	case tagEnd:
		return nil
	case tagByte:
		return n.discard(1)
	case tagShort:
		return n.discard(2)
	case tagInt, tagFloat:
		return n.discard(4)
	case tagLong, tagDouble:
		return n.discard(8)
	case tagByteArray:
		count, err := n.readInt32()
		if err != nil {
			return err
		}
		if count < 0 || count > maxNBTBytes {
			return errors.New("NBT 字节数组长度无效")
		}
		return n.discard(int64(count))
	case tagString:
		var b [2]byte
		if err := n.readFull(b[:]); err != nil {
			return err
		}
		length := int64(binary.BigEndian.Uint16(b[:]))
		if length > maxNBTString {
			return errors.New("NBT 字符串超出大小限制")
		}
		return n.discard(length)
	case tagList:
		elemType, err := n.readByte()
		if err != nil {
			return err
		}
		count, err := n.readInt32()
		if err != nil {
			return err
		}
		if count < 0 || count > maxNBTCollection {
			return errors.New("NBT 列表长度无效")
		}
		for range count {
			if err := n.skipPayload(elemType, depth+1); err != nil {
				return err
			}
		}
		return nil
	case tagCompound:
		for {
			childType, err := n.readByte()
			if err != nil {
				return err
			}
			if childType == tagEnd {
				return nil
			}
			if _, err := n.readString(); err != nil {
				return err
			}
			if err := n.skipPayload(childType, depth+1); err != nil {
				return err
			}
		}
	case tagIntArray:
		count, err := n.readInt32()
		if err != nil {
			return err
		}
		if count < 0 || count > maxNBTCollection*64 {
			return errors.New("NBT 整数数组长度无效")
		}
		return n.discard(int64(count) * 4)
	case tagLongArray:
		count, err := n.readInt32()
		if err != nil {
			return err
		}
		if count < 0 || count > maxNBTCollection*64 {
			return errors.New("NBT 长整数数组长度无效")
		}
		return n.discard(int64(count) * 8)
	default:
		return fmt.Errorf("未知 NBT 标签类型 %d", tagType)
	}
}

func (n *nbtReader) readServerList(depth int) ([]DiscoveredServer, error) {
	if depth > maxNBTDepth {
		return nil, errors.New("NBT 嵌套层级过深")
	}
	elemType, err := n.readByte()
	if err != nil {
		return nil, err
	}
	count, err := n.readInt32()
	if err != nil {
		return nil, err
	}
	if count < 0 || count > maxNBTCollection {
		return nil, errors.New("servers.dat 列表数量超出限制")
	}
	if elemType != tagCompound {
		for range count {
			if err := n.skipPayload(elemType, depth+1); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	servers := make([]DiscoveredServer, 0, min(int(count), 64))
	for range count {
		entry, valid, err := n.readServerCompound(depth + 1)
		if err != nil {
			return nil, err
		}
		if valid {
			servers = append(servers, entry)
		}
	}
	return servers, nil
}

func (n *nbtReader) readServerCompound(depth int) (DiscoveredServer, bool, error) {
	if depth > maxNBTDepth {
		return DiscoveredServer{}, false, errors.New("NBT 嵌套层级过深")
	}
	var rawName, rawIP, rawIcon string
	var acceptTextures, hidden bool
	for {
		tagType, err := n.readByte()
		if err != nil {
			return DiscoveredServer{}, false, err
		}
		if tagType == tagEnd {
			break
		}
		field, err := n.readString()
		if err != nil {
			return DiscoveredServer{}, false, err
		}
		switch {
		case field == "name" && tagType == tagString:
			rawName, err = n.readString()
		case field == "ip" && tagType == tagString:
			rawIP, err = n.readString()
		case field == "icon" && tagType == tagString:
			rawIcon, err = n.readString()
		case field == "acceptTextures" && tagType == tagByte:
			var b byte
			b, err = n.readByte()
			acceptTextures = b != 0
		case field == "hidden" && tagType == tagByte:
			var b byte
			b, err = n.readByte()
			hidden = b != 0
		default:
			err = n.skipPayload(tagType, depth+1)
		}
		if err != nil {
			return DiscoveredServer{}, false, err
		}
	}
	key, display, _, _, err := NormalizeAddress(rawIP)
	if err != nil {
		return DiscoveredServer{}, false, nil
	}
	name := SanitizeName(rawName)
	if name == "" {
		name = display
	}
	icon := NormalizeServerIconBase64(rawIcon)
	return DiscoveredServer{
		Name:           name,
		Address:        display,
		Key:            key,
		Icon:           icon,
		AcceptTextures: acceptTextures,
		Hidden:         hidden,
	}, true, nil
}

// SanitizeName strips control characters and bounds server display names.
func SanitizeName(value string) string {
	value = strings.TrimSpace(strings.ToValidUTF8(value, ""))
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
	if utf8.RuneCountInString(value) > 120 {
		runes := []rune(value)
		value = string(runes[:120])
	}
	return strings.TrimSpace(value)
}

// NormalizeServerIconBase64 decodes an encoded PNG icon (raw base64 or data URI)
// and re-encodes a bounded 80x80 PNG thumbnail. Invalid icons return nil.
func NormalizeServerIconBase64(raw string) []byte {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > thumbnail.MaxBytes*2 {
		return nil
	}
	if _, after, ok := strings.Cut(raw, "base64,"); ok {
		raw = after
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(raw)
		if err != nil {
			return nil
		}
	}
	return normalizeServerIconBytes(decoded)
}

func normalizeServerIconBytes(data []byte) []byte {
	img, err := thumbnail.Decode(data)
	if err != nil {
		return nil
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil || out.Len() > maxIconBytes {
		return nil
	}
	return out.Bytes()
}
