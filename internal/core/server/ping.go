package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	slpProtocolVersion = 767
	slpMaxPacketBytes  = 512 << 10
	slpDialTimeout     = 4 * time.Second
	slpIOTimeout       = 5 * time.Second
)

// Status describes the live result of a Minecraft Server List Ping query.
type Status struct {
	Online        bool       `json:"online"`
	LatencyMS     int        `json:"latency_ms"`
	Version       string     `json:"version,omitempty"`
	Protocol      int        `json:"protocol,omitempty"`
	OnlinePlayers int        `json:"online_players"`
	MaxPlayers    int        `json:"max_players"`
	SamplePlayers []string   `json:"sample_players,omitempty"`
	MOTD          []MOTDLine `json:"motd,omitempty"`
	PlainMOTD     string     `json:"plain_motd,omitempty"`
	Favicon       []byte     `json:"favicon,omitempty"`
	CheckedAt     time.Time  `json:"checked_at"`
}

// Ping queries a Minecraft Java Edition server using Server List Ping.
func Ping(ctx context.Context, rawAddress string) (Status, error) {
	_, _, host, port, err := NormalizeAddress(rawAddress)
	if err != nil {
		return Status{}, err
	}
	connectHost, connectPort := host, port
	if port == 25565 && net.ParseIP(host) == nil {
		srvCtx, cancelSRV := context.WithTimeout(ctx, 1500*time.Millisecond)
		_, addrs, srvErr := net.DefaultResolver.LookupSRV(srvCtx, "minecraft", "tcp", host)
		cancelSRV()
		if srvErr == nil && len(addrs) > 0 {
			target := strings.TrimSuffix(strings.TrimSpace(addrs[0].Target), ".")
			if target != "" && addrs[0].Port > 0 {
				connectHost = target
				connectPort = int(addrs[0].Port)
			}
		}
	}

	dialer := net.Dialer{Timeout: slpDialTimeout}
	dialStart := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(connectHost, strconv.Itoa(connectPort)))
	if err != nil {
		return Status{}, err
	}
	defer conn.Close()

	stopAbort := context.AfterFunc(ctx, func() {
		_ = conn.SetDeadline(time.Now())
	})
	defer stopAbort()

	deadline := time.Now().Add(slpIOTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return Status{}, err
	}

	handshake := buildHandshakePacket(slpProtocolVersion, connectHost, uint16(connectPort), 1)
	if _, err := conn.Write(handshake); err != nil {
		return Status{}, err
	}
	statusReq := []byte{0x01, 0x00}
	statusStart := time.Now()
	if _, err := conn.Write(statusReq); err != nil {
		return Status{}, err
	}

	payload, err := readPacket(conn)
	if err != nil {
		return Status{}, err
	}
	packetID, rest, err := readVarInt(payload)
	if err != nil {
		return Status{}, err
	}
	if packetID != 0x00 {
		return Status{}, fmt.Errorf("服务器返回了意外的状态包 ID %d", packetID)
	}
	jsonLen, rest, err := readVarInt(rest)
	if err != nil || jsonLen < 0 || jsonLen > len(rest) {
		return Status{}, errors.New("服务器状态响应长度无效")
	}
	statusData := rest[:jsonLen]

	var decoded struct {
		Version struct {
			Name     string `json:"name"`
			Protocol int    `json:"protocol"`
		} `json:"version"`
		Players struct {
			Max    int `json:"max"`
			Online int `json:"online"`
			Sample []struct {
				Name string `json:"name"`
			} `json:"sample"`
		} `json:"players"`
		Description json.RawMessage `json:"description"`
		Favicon     string          `json:"favicon"`
	}
	if err := json.Unmarshal(statusData, &decoded); err != nil {
		return Status{}, fmt.Errorf("解析服务器状态失败: %w", err)
	}

	latencyMS := int(time.Since(statusStart).Milliseconds())
	if pingRTT, ok := measurePingRTT(conn); ok {
		latencyMS = pingRTT
	} else if latencyMS <= 0 {
		latencyMS = int(time.Since(dialStart).Milliseconds())
	}
	if latencyMS < 1 {
		latencyMS = 1
	}

	lines, plain := ParseMOTD(decoded.Description)
	var samples []string
	for _, sample := range decoded.Players.Sample {
		if len(samples) >= 12 {
			break
		}
		name := stripSectionCodes(SanitizeName(sample.Name))
		if name != "" {
			samples = append(samples, name)
		}
	}

	return Status{
		Online:        true,
		LatencyMS:     latencyMS,
		Version:       stripSectionCodes(SanitizeName(decoded.Version.Name)),
		Protocol:      decoded.Version.Protocol,
		OnlinePlayers: max(0, decoded.Players.Online),
		MaxPlayers:    max(0, decoded.Players.Max),
		SamplePlayers: samples,
		MOTD:          lines,
		PlainMOTD:     plain,
		Favicon:       NormalizeServerIconBase64(decoded.Favicon),
		CheckedAt:     time.Now().UTC(),
	}, ctx.Err()
}

func measurePingRTT(conn net.Conn) (int, bool) {
	nonce := time.Now().UnixNano()
	var body [9]byte
	body[0] = 0x01
	binary.BigEndian.PutUint64(body[1:], uint64(nonce))
	var packet [10]byte
	packet[0] = 0x09
	copy(packet[1:], body[:])

	start := time.Now()
	if _, err := conn.Write(packet[:]); err != nil {
		return 0, false
	}
	resp, err := readPacket(conn)
	if err != nil {
		return 0, false
	}
	packetID, rest, err := readVarInt(resp)
	if err != nil || packetID != 0x01 || len(rest) != 8 {
		return 0, false
	}
	return int(time.Since(start).Milliseconds()), true
}

func buildHandshakePacket(protocol int, host string, port uint16, nextState int) []byte {
	var body bytes.Buffer
	writeVarInt(&body, 0x00)
	writeVarInt(&body, protocol)
	writeVarInt(&body, len(host))
	body.WriteString(host)
	var portBytes [2]byte
	binary.BigEndian.PutUint16(portBytes[:], port)
	body.Write(portBytes[:])
	writeVarInt(&body, nextState)

	var frame bytes.Buffer
	writeVarInt(&frame, body.Len())
	frame.Write(body.Bytes())
	return frame.Bytes()
}

func writeVarInt(buf *bytes.Buffer, value int) {
	u := uint32(value)
	for {
		if u&^0x7F == 0 {
			buf.WriteByte(byte(u))
			return
		}
		buf.WriteByte(byte(u&0x7F) | 0x80)
		u >>= 7
	}
}

func readStreamVarInt(r io.Reader) (int, error) {
	var numRead int
	var result uint32
	var b [1]byte
	for {
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, err
		}
		val := b[0]
		result |= uint32(val&0x7F) << (7 * numRead)
		numRead++
		if numRead > 5 {
			return 0, errors.New("VarInt 超出长度")
		}
		if val&0x80 == 0 {
			break
		}
	}
	return int(int32(result)), nil
}

func readVarInt(data []byte) (int, []byte, error) {
	var result uint32
	for i := range min(len(data), 5) {
		b := data[i]
		result |= uint32(b&0x7F) << (7 * i)
		if b&0x80 == 0 {
			return int(int32(result)), data[i+1:], nil
		}
	}
	return 0, nil, errors.New("数据包 VarInt 无效")
}

func readPacket(r io.Reader) ([]byte, error) {
	length, err := readStreamVarInt(r)
	if err != nil {
		return nil, err
	}
	if length <= 0 || length > slpMaxPacketBytes {
		return nil, errors.New("服务器响应包大小超出限制")
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func stripSectionCodes(value string) string {
	runes := []rune(value)
	var out strings.Builder
	for i := 0; i < len(runes); i++ {
		if runes[i] == '§' && i+1 < len(runes) {
			if unicode.ToLower(runes[i+1]) == 'x' && i+13 < len(runes) {
				if _, ok := parseLegacyHex(runes[i+2:]); ok {
					i += 13
					continue
				}
			}
			i++
			continue
		}
		out.WriteRune(runes[i])
	}
	return strings.TrimSpace(out.String())
}
