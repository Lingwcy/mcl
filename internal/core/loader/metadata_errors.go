package loader

import (
	"errors"
	"mcl/internal/core/diagnostics"
)

type mavenChecksumError struct{ cause error }

func (e *mavenChecksumError) Error() string { return e.cause.Error() }
func (e *mavenChecksumError) Unwrap() error { return e.cause }

var errMetadataBudget = errors.New("加载器元数据解析预算已用尽，请核对诊断后重试")

func identityError(reason string) error {
	return diagnostics.Wrap("artifact-identity", errors.New(reason))
}
