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

// mirrorURL deliberately excludes platform files, signed query URLs, credentials
// and arbitrary Maven repositories. The fixed Temurin archive mapping is the
// only Java mirror route. Callers must already hold an independent pinned digest.
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
	mirror := mirrorURL(original)
	if c == nil || mirror == "" {
		return []string{original}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	switch c.preferences.Source {
	case SourceMirrorFirst:
		return []string{mirror, original}
	case SourceAuto:
		u, _ := url.Parse(original)
		if recent := c.preferred[strings.ToLower(u.Hostname())]; recent.mirror && time.Now().Before(recent.until) {
			return []string{mirror, original}
		}
		return []string{original, mirror}
	case SourceOfficialFirst:
		return []string{original, mirror}
	default:
		return []string{original}
	}
}

func (c *TransferControl) rememberSource(original, successful string) {
	if c == nil || mirrorURL(original) == "" {
		return
	}
	u, _ := url.Parse(original)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.preferences.Source == SourceAuto {
		c.preferred[strings.ToLower(u.Hostname())] = sourceHealth{mirror: successful != original, until: time.Now().Add(10 * time.Minute)}
	}
}

// Mirror redirects may delegate to public HTTPS CDN nodes. They never receive
// credentials and still have to satisfy the original size and digest. Literal
// private/local targets and HTTPS downgrades are rejected before any request.
func safeMirrorRedirect(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Hostname() == "" {
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
