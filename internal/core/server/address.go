// Package server handles Minecraft multiplayer server addresses, servers.dat
// NBT files, MOTD formatting, and Server List Ping status queries.
package server

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/idna"
)

var errInvalidAddress = errors.New("请输入有效的服务器地址和可选端口，例如 mc.example.com:25565 或 [::1]:25565")

// NormalizeAddress validates a Minecraft server address and returns its
// canonical deduplication key (lowerHost:port), human display form (omitting
// the default 25565 port), normalized host, and TCP port.
func NormalizeAddress(raw string) (key, display, host string, port int, err error) {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > 512 || !utf8.ValidString(raw) || strings.IndexFunc(raw, unicode.IsControl) >= 0 || strings.ContainsAny(value, "/\\@?# \t") {
		return "", "", "", 0, errInvalidAddress
	}
	host, port = value, 25565
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		host = value[1 : len(value)-1]
		if net.ParseIP(host) == nil {
			return "", "", "", 0, errInvalidAddress
		}
	} else if strings.Count(value, ":") == 1 || strings.HasPrefix(value, "[") {
		bracketed := strings.HasPrefix(value, "[")
		var rawPort string
		host, rawPort, err = net.SplitHostPort(value)
		if err != nil {
			return "", "", "", 0, errInvalidAddress
		}
		port, err = strconv.Atoi(rawPort)
		if err != nil || port < 1 || port > 65535 {
			return "", "", "", 0, errInvalidAddress
		}
		if bracketed && net.ParseIP(host) == nil {
			return "", "", "", 0, errInvalidAddress
		}
	} else if strings.Contains(value, ":") && net.ParseIP(value) == nil {
		return "", "", "", 0, errInvalidAddress
	}

	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	} else {
		host, err = idna.Lookup.ToASCII(host)
		if err != nil || len(host) > 253 || host == "" {
			return "", "", "", 0, errInvalidAddress
		}
		host = strings.ToLower(strings.TrimSuffix(host, "."))
		if host == "" {
			return "", "", "", 0, errInvalidAddress
		}
		for _, label := range strings.Split(host, ".") {
			if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return "", "", "", 0, errInvalidAddress
			}
			for _, char := range label {
				if char != '-' && char != '_' && !(char >= 'a' && char <= 'z') && !(char >= '0' && char <= '9') {
					return "", "", "", 0, errInvalidAddress
				}
			}
		}
	}

	key = net.JoinHostPort(host, strconv.Itoa(port))
	if port == 25565 {
		if strings.Contains(host, ":") {
			display = "[" + host + "]"
		} else {
			display = host
		}
	} else {
		display = key
	}
	return key, display, host, port, nil
}
