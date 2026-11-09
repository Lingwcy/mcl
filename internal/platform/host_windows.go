package platform

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

func operatingSystemVersion() (string, error) {
	// Avoid manifest-dependent GetVersionEx compatibility values. Java's
	// os.version on Windows uses major.minor, not the marketing name/build.
	major, minor, _ := windows.RtlGetNtVersionNumbers()
	if major == 0 {
		return "", errors.New("Windows未返回有效系统版本")
	}
	return fmt.Sprintf("%d.%d", major, minor), nil
}
