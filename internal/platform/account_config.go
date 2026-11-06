package platform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"mcl/internal/fileio"
)

// MicrosoftClientID reads only the application's client ID. The local file
// is parsed as data and is never sourced or added to the process environment.
func MicrosoftClientID(ctx context.Context, dataRoot string) (string, error) {
	const key = "MCL_MICROSOFT_CLIENT_ID"
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value, nil
	}
	data, err := fileio.Read(ctx, filepath.Join(dataRoot, ".env.local"), 16<<10)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var clientID string
	found := false
	for line := range strings.SplitSeq(string(data), "\n") {
		name, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(name) != key {
			continue
		}
		if found {
			return "", errors.New("MCL_MICROSOFT_CLIENT_ID appears more than once in .env.local")
		}
		found = true
		clientID = strings.TrimSpace(value)
		if n := len(clientID); n >= 2 && (clientID[0] == '"' || clientID[0] == '\'') && clientID[0] == clientID[n-1] {
			clientID = clientID[1 : n-1]
		}
	}
	return clientID, nil
}
