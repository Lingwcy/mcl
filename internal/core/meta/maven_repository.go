package meta

import (
	"errors"
	"net/url"
	"strings"
)

// MavenLocation binds an exact repository and coordinate. Repository aliases
// do not substitute versions/classifiers and are not proof of matching bytes.
type MavenLocation struct {
	Repository string
	Coordinate Coordinate
}

func mavenRoot(repository string) string {
	switch repository {
	case "minecraft":
		return "https://libraries.minecraft.net/"
	case "forge":
		return "https://maven.minecraftforge.net/"
	case "neoforge":
		return "https://maven.neoforged.net/releases/"
	case "fabric":
		return "https://maven.fabricmc.net/"
	case "quilt-release":
		return "https://maven.quiltmc.org/repository/release/"
	case "central":
		return "https://repo.maven.apache.org/maven2/"
	case "central-repo1":
		return "https://repo1.maven.org/maven2/"
	}
	return ""
}
func MavenRepositoryRoot(address string) (string, error) {
	// A missing repository is the historical Mojang library default. Callers
	// must distinguish omitted JSON fields from explicit null/empty values.
	if address == "" {
		return mavenRoot("minecraft"), nil
	}
	normalized := strings.TrimSuffix(address, "/") + "/"
	for _, repository := range []string{"minecraft", "forge", "neoforge", "fabric", "quilt-release", "central", "central-repo1"} {
		if normalized == mavenRoot(repository) {
			return normalized, nil
		}
	}
	return "", errors.New("Maven仓库根不在固定允许列表中")
}
func (l MavenLocation) URL() string {
	root := mavenRoot(l.Repository)
	if root == "" {
		return ""
	}
	return root + l.Coordinate.Path()
}
func BindMavenURL(address string, coordinate Coordinate) (MavenLocation, error) {
	location, err := ParseMavenURL(address)
	if err != nil {
		return MavenLocation{}, err
	}
	if location.Coordinate != coordinate {
		return MavenLocation{}, errors.New("Maven地址与声明坐标不一致")
	}
	return location, nil
}
func ParseMavenURL(address string) (MavenLocation, error) {
	invalid := errors.New("Maven制品地址或仓库路径无效")
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" {
		return MavenLocation{}, invalid
	}
	for _, repository := range []string{"minecraft", "forge", "neoforge", "fabric", "quilt-release", "central", "central-repo1"} {
		root := mavenRoot(repository)
		// Canonical HTTPS roots reject port/userinfo, encoded path aliases and
		// prefixes such as /arbitrary/repo, even on an otherwise trusted host.
		if !strings.HasPrefix(address, root) {
			continue
		}
		relative := strings.TrimPrefix(address, root)
		coordinate, err := CoordinateFromPath(relative)
		if err != nil {
			return MavenLocation{}, invalid
		}
		location := MavenLocation{Repository: repository, Coordinate: coordinate}
		if location.URL() != address {
			return MavenLocation{}, invalid
		}
		return location, nil
	}
	return MavenLocation{}, invalid
}
func CoordinateFromPath(relative string) (Coordinate, error) {
	if !ValidPath(relative) {
		return Coordinate{}, errors.New("Maven相对路径无效")
	}
	parts := strings.Split(relative, "/")
	if len(parts) < 4 {
		return Coordinate{}, errors.New("Maven路径缺少组或版本")
	}
	group := strings.Join(parts[:len(parts)-3], ".")
	artifact, version, name := parts[len(parts)-3], parts[len(parts)-2], parts[len(parts)-1]
	dot := strings.LastIndexByte(name, '.')
	if dot <= 0 || dot == len(name)-1 {
		return Coordinate{}, errors.New("Maven文件扩展名无效")
	}
	stem, ext := name[:dot], name[dot+1:]
	prefix := artifact + "-" + version
	classifier := ""
	if stem != prefix {
		if !strings.HasPrefix(stem, prefix+"-") {
			return Coordinate{}, errors.New("Maven文件名与目录身份不匹配")
		}
		classifier = strings.TrimPrefix(stem, prefix+"-")
		if classifier == "" {
			return Coordinate{}, errors.New("Maven分类为空")
		}
	}
	declaration := group + ":" + artifact + ":" + version
	if classifier != "" {
		declaration += ":" + classifier
	}
	declaration += "@" + ext
	coordinate, err := ParseCoordinate(declaration)
	if err != nil || coordinate.Path() != relative {
		return Coordinate{}, errors.New("Maven坐标与路径不一致")
	}
	return coordinate, nil
}
