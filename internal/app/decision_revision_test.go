package app

import (
	"errors"
	"strings"
	"surge-controller/internal/config"
	"testing"
)

func TestDecisionRevisionKeepsLastConfirmedVersionThroughReadFailure(t *testing.T) {
	for _, invalidate := range []bool{false, true} {
		r := decisionRevision{}
		c := config.Default().Settings.Cache
		c.InvalidateOnReadFailure = invalidate
		r.observe("profile-a", "node-a", nil, c)
		if reason, _ := r.observe("", "", errors.New("failed"), c); (reason != "") != invalidate {
			t.Fatal("wrong failure behavior", reason)
		}
		if reason, _ := r.observe("", "", errors.New("failed again"), c); reason != "" {
			t.Fatal("repeated failure flushed cache")
		}
		if reason, _ := r.observe("profile-a", "node-a", nil, c); reason != "" {
			t.Fatal("unchanged recovery flushed cache")
		}
		r.observe("", "", errors.New("failed"), c)
		if reason, selection := r.observe("profile-b", "node-a", nil, c); reason == "" || selection {
			t.Fatal("confirmed change after failed read was missed")
		}
	}
}
func TestDecisionRevisionSelectionPolicyIsIndependent(t *testing.T) {
	for _, direct := range []bool{false, true} {
		for _, reject := range []bool{false, true} {
			c := config.Default().Settings.Cache
			c.InvalidateDirectOnSelectionChange = direct
			c.InvalidateRejectOnSelectionChange = reject
			r := decisionRevision{}
			r.observe("profile", "node-a", nil, c)
			reason, selection := r.observe("profile", "node-b", nil, c)
			if selection != (direct || reject) || strings.Contains(reason, "DIRECT") != direct || strings.Contains(reason, "REJECT") != reject {
				t.Fatal("wrong selection policy", reason)
			}
			if reason, _ = r.observe("profile", "node-b", nil, c); reason != "" {
				t.Fatal("same selection invalidated twice")
			}
			if reason, selection = r.observe("changed", "node-b", nil, c); reason == "" || selection {
				t.Fatal("configuration update must invalidate all decisions")
			}
		}
	}
}
