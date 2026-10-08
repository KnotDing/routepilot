package app

import (
	"strings"
	"surge-controller/internal/config"
)

// Keep the last confirmed revision through failed reads. Recovery alone does
// not establish that configuration changed.
type decisionRevision struct {
	digest, selection string
	failed            bool
}

func (r *decisionRevision) observe(digest, selection string, err error, c config.Cache) (string, bool) {
	if err != nil || digest == "" {
		firstFailure := !r.failed
		r.failed = true
		if firstFailure && c.InvalidateOnReadFailure {
			return "Surge 配置读取失败；按配置失效旧决策", false
		}
		return "", false
	}
	r.failed = false
	previous, previousSelection := r.digest, r.selection
	r.digest, r.selection = digest, selection
	if digest != previous {
		if previous == "" {
			return "Surge 配置已首次确认", false
		}
		return "Surge 有效配置或外部资源已确认更新", false
	}
	if selection != previousSelection {
		var policies []string
		if c.InvalidateDirectOnSelectionChange {
			policies = append(policies, "DIRECT")
		}
		if c.InvalidateRejectOnSelectionChange {
			policies = append(policies, "REJECT")
		}
		if len(policies) > 0 {
			return "Surge 策略组选择已变化；按配置仅失效 " + strings.Join(policies, "/"), true
		}
	}
	return "", false
}
