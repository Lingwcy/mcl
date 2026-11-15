package download

import (
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"mcl/internal/core/meta"
)

const mirrorOrigin = "https://bmclapi2.bangbang93.com"

var temurinArchive = regexp.MustCompile(`^OpenJDK([0-9]+)U-(jdk|jre)_(x64|aarch64|x86-32)_(mac|linux|windows)_hotspot_([A-Za-z0-9._+-]+)\.(tar\.gz|zip)$`)

type sourceHealth struct {
	mirror bool
	until  time.Time
}

// mirrorURL excludes signed query URLs, credentials and arbitrary Maven
// repositories. Only existing Mojang routes, fixed Forge distributions and
// fixed Temurin archives are mapped. Callers hold an independent pinned digest.
func mirrorURL(original string) string {
	u, err := url.Parse(original)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || !meta.ValidPath(strings.TrimPrefix(u.Path, "/")) {
		return ""
	}
	if strings.EqualFold(u.Hostname(), "github.com") {
		parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
		if len(parts) == 6 && parts[0] == "adoptium" && parts[2] == "releases" && parts[3] == "download" {
			majorText := strings.TrimSuffix(strings.TrimPrefix(parts[1], "temurin"), "-binaries")
			major, err := strconv.Atoi(majorText)
			matches := temurinArchive.FindStringSubmatch(path.Base(u.Path))
			if err == nil && parts[1] == "temurin"+majorText+"-binaries" && major >= 8 && major <= 100 && len(matches) == 7 && matches[1] == strconv.Itoa(major) && ((matches[4] == "windows" && matches[6] == "zip") || (matches[4] != "windows" && matches[6] == "tar.gz")) {
				return "https://mirrors.tuna.tsinghua.edu.cn/Adoptium/" + majorText + "/" + matches[2] + "/" + matches[3] + "/" + matches[4] + "/" + parts[5]
			}
		}
		return ""
	}
	if u.RawPath != "" {
		return ""
	}
	if fixed := mavenMirrorURL(original); fixed != "" {
		return fixed
	}
	switch strings.ToLower(u.Hostname()) {
	case "resources.download.minecraft.net":
		return mirrorOrigin + "/assets" + u.Path
	case "libraries.minecraft.net":
		return mirrorOrigin + "/maven" + u.Path
	case "piston-data.mojang.com", "piston-meta.mojang.com", "launcher.mojang.com", "launchermeta.mojang.com":
		if !strings.HasPrefix(u.Path, "/v1/") {
			return ""
		}
		return mirrorOrigin + u.Path
	}
	return ""
}

func (c *TransferControl) candidates(original string) []string {
	official := []string{original}
	if alternate := centralAlternate(original); alternate != "" {
		official = append(official, alternate)
	}
	mirror := mirrorURL(original)
	if c == nil || mirror == "" {
		return official
	}
	return c.orderCandidates(official, []string{mirror})
}

func (c *TransferControl) orderCandidates(official, mirrors []string) []string {
	if c == nil || len(mirrors) == 0 || len(official) == 0 {
		return official
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	switch c.preferences.Source {
	case SourceMirrorFirst:
		out := make([]string, 0, len(mirrors)+len(official))
		out = append(out, mirrors...)
		return append(out, official...)
	case SourceAuto:
		u, _ := url.Parse(official[0])
		if recent := c.preferred[strings.ToLower(u.Hostname())]; recent.mirror && time.Now().Before(recent.until) {
			out := make([]string, 0, len(mirrors)+len(official))
			out = append(out, mirrors...)
			return append(out, official...)
		}
		out := make([]string, 0, len(official)+len(mirrors))
		out = append(out, official...)
		return append(out, mirrors...)
	case SourceOfficialFirst:
		out := make([]string, 0, len(official)+len(mirrors))
		out = append(out, official...)
		return append(out, mirrors...)
	default:
		return official
	}
}

func (c *TransferControl) rememberSource(original, successful string) {
	if c == nil || mirrorURL(original) == "" {
		return
	}
	u, _ := url.Parse(original)
	mirrored := successful != original && successful != centralAlternate(original)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.preferences.Source == SourceAuto {
		c.preferred[strings.ToLower(u.Hostname())] = sourceHealth{mirror: mirrored, until: time.Now().Add(10 * time.Minute)}
	}
}

// SourceMode returns the active source selection policy.
func (c *TransferControl) SourceMode() SourceMode {
	if c == nil {
		return SourceOfficial
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.preferences.Source
}

func (c *TransferControl) hasSpeedLimit() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.preferences.SpeedLimitKiB > 0
}

// OrderURLs orders official and mirror candidates according to the active source
// policy while reusing recent automatic mirror health per official host.
func (c *TransferControl) OrderURLs(official string, mirrors ...string) []string {
	return c.orderCandidates([]string{official}, mirrors)
}

// RememberCandidate updates automatic source health after a metadata request.
func (c *TransferControl) RememberCandidate(official, successful string) {
	if c == nil || official == "" || successful == "" {
		return
	}
	u, err := url.Parse(official)
	if err != nil || u.Hostname() == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.preferences.Source == SourceAuto {
		c.preferred[strings.ToLower(u.Hostname())] = sourceHealth{mirror: successful != official, until: time.Now().Add(10 * time.Minute)}
	}
}

// SafeMirrorRedirect reports whether a mirror redirect target is a public HTTPS host.
func SafeMirrorRedirect(u *url.URL) bool {
	return safeMirrorRedirect(u)
}

// Mirror redirects may delegate to public HTTPS CDN nodes. They never receive
// credentials and still have to satisfy the original size and digest. Literal
// private/local targets and HTTPS downgrades are rejected before any request.
func safeMirrorRedirect(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") || u.Hostname() == "" {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast()) {
		return false
	}
	return true
}

func stripTransferSecrets(request *http.Request) {
	for _, header := range []string{"Authorization", "Proxy-Authorization", "Cookie", "X-API-Key", "Referer"} {
		request.Header.Del(header)
	}
}
