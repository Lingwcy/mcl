package meta

import (
	"fmt"
	"strings"
)

type Coordinate struct {
	Group      string
	Artifact   string
	Version    string
	Classifier string
	Extension  string
}

func ParseCoordinate(value string) (Coordinate, error) {
	base, ext, hasExt := strings.Cut(value, "@")
	if !hasExt {
		ext = "jar"
	}
	parts := strings.Split(base, ":")
	if (len(parts) != 3 && len(parts) != 4) || !ValidID(ext) || strings.Contains(ext, "@") {
		return Coordinate{}, fmt.Errorf("invalid Maven coordinate %q", value)
	}
	for _, part := range parts {
		if !ValidID(part) || strings.ContainsAny(part, "@ ") {
			return Coordinate{}, fmt.Errorf("invalid Maven coordinate %q", value)
		}
	}
	for part := range strings.SplitSeq(parts[0], ".") {
		if !ValidID(part) {
			return Coordinate{}, fmt.Errorf("invalid Maven group %q", parts[0])
		}
	}
	coordinate := Coordinate{Group: parts[0], Artifact: parts[1], Version: parts[2], Extension: ext}
	if len(parts) == 4 {
		coordinate.Classifier = parts[3]
	}
	return coordinate, nil
}

func (c Coordinate) Path() string {
	name := c.Artifact + "-" + c.Version
	if c.Classifier != "" {
		name += "-" + c.Classifier
	}
	return strings.ReplaceAll(c.Group, ".", "/") + "/" + c.Artifact + "/" + c.Version + "/" + name + "." + c.Extension
}
