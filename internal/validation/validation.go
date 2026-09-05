// Package validation performs strict validation of the account JSON provided
// through the environment. An invalid configuration is never silently repaired:
// it is reported with an explicit error and no account is processed.
package validation

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Account is the manager-level account definition. It mirrors the credentials
// expected by FlareTunnel (name, api_token, account_id, optional zone_id) plus
// the manager-level target_workers objective used by MODE=create and MODE=delete.
type Account struct {
	Name          string `json:"name"`
	APIToken      string `json:"api_token"`
	AccountID     string `json:"account_id"`
	ZoneID        string `json:"zone_id,omitempty"`
	TargetWorkers *int   `json:"target_workers,omitempty"`
}

// Validate parses and strictly validates the accounts JSON for the active mode.
//
// requireTarget must be true for MODE=create and MODE=delete, where every account
// must declare its target_workers objective.
//
// It returns an explicit error (and no accounts) when the JSON is invalid,
// empty while an account is required, of the wrong type, or contains an
// account that does not conform to the expected structure.
func Validate(raw string, requireTarget bool) ([]Account, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("accounts JSON is empty; at least one account is required")
	}

	var accounts []Account
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&accounts); err != nil {
		return nil, fmt.Errorf("accounts JSON is invalid: %v", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("accounts JSON is invalid: trailing data")
		}
		return nil, fmt.Errorf("accounts JSON is invalid: %v", err)
	}

	if len(accounts) == 0 {
		return nil, fmt.Errorf("accounts JSON is an empty list; at least one account is required")
	}

	seen := make(map[string]bool, len(accounts))
	for i, acc := range accounts {
		prefix := fmt.Sprintf("account[%d]", i)
		if strings.TrimSpace(acc.Name) == "" {
			return nil, fmt.Errorf("%s: missing or empty required field 'name'", prefix)
		}
		if strings.TrimSpace(acc.APIToken) == "" {
			return nil, fmt.Errorf("%s (%s): missing or empty required field 'api_token'", prefix, acc.Name)
		}
		if strings.TrimSpace(acc.AccountID) == "" {
			return nil, fmt.Errorf("%s (%s): missing or empty required field 'account_id'", prefix, acc.Name)
		}
		if seen[acc.Name] {
			return nil, fmt.Errorf("duplicate account name %q", acc.Name)
		}
		seen[acc.Name] = true

		if requireTarget {
			if acc.TargetWorkers == nil {
				return nil, fmt.Errorf("%s (%s): missing required field 'target_workers'", prefix, acc.Name)
			}
			if *acc.TargetWorkers < 0 {
				return nil, fmt.Errorf("%s (%s): 'target_workers' must be >= 0 (got %d)", prefix, acc.Name, *acc.TargetWorkers)
			}
		}
	}

	return accounts, nil
}
