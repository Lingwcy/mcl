package launch

import "mcl/internal/core/java"

func encodeArgumentFile(arguments []string) ([]byte, error) {
	return java.EncodeArgumentFile(arguments)
}
