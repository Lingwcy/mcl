package launch

import "errors"

// ProcessPriority is applied only to a newly tracked game before its handoff.
// The zero value is normal. No real-time class or elevation is supported.
type ProcessPriority string

const (
	PriorityNormal      ProcessPriority = ""
	PriorityLow         ProcessPriority = "low"
	PriorityBelowNormal ProcessPriority = "below-normal"
	PriorityAboveNormal ProcessPriority = "above-normal"
	PriorityHigh        ProcessPriority = "high"
)

func (p ProcessPriority) Validate() error {
	switch p {
	case PriorityNormal, PriorityLow, PriorityBelowNormal, PriorityAboveNormal, PriorityHigh:
		return nil
	default:
		return errors.New("游戏进程优先级无效")
	}
}

func (p ProcessPriority) Label() string {
	switch p {
	case PriorityNormal:
		return "正常"
	case PriorityLow:
		return "低"
	case PriorityBelowNormal:
		return "低于正常"
	case PriorityAboveNormal:
		return "高于正常"
	case PriorityHigh:
		return "高"
	default:
		return "未知"
	}
}

// StartupWarnings returns immutable, already-public adapter notices when
// supported. Synthetic games and older adapters need not implement the optional
// reporter. These notices are never process output or command-line arguments.
func StartupWarnings(game Game) []string {
	if reporter, ok := game.(interface{ StartupWarnings() []string }); ok {
		return append([]string(nil), reporter.StartupWarnings()...)
	}
	return nil
}
