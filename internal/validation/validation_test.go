package validation

import (
	"strings"
	"testing"
)

const validCreate = `[
  {"name":"acc-a","api_token":"tok-a","account_id":"id-a","target_workers":100},
  {"name":"acc-b","api_token":"tok-b","account_id":"id-b","target_workers":50}
]`

func TestValidateValidCreate(t *testing.T) {
	accs, err := Validate(validCreate, true)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if len(accs) != 2 {
		t.Fatalf("got %d accounts, want 2", len(accs))
	}
	if accs[0].Name != "acc-a" || *accs[0].TargetWorkers != 100 {
		t.Errorf("unexpected account[0]: %+v", accs[0])
	}
}

func TestValidateEmptyString(t *testing.T) {
	if _, err := Validate("", true); err == nil {
		t.Fatal("expected error for empty JSON")
	}
	if _, err := Validate("   ", false); err == nil {
		t.Fatal("expected error for whitespace JSON")
	}
}

func TestValidateInvalidJSON(t *testing.T) {
	_, err := Validate(`{"name":`, true)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("error should mention invalid JSON, got: %v", err)
	}
}

func TestValidateWrongType(t *testing.T) {
	if _, err := Validate(`{"accounts":[]}`, true); err == nil {
		t.Fatal("expected error for object instead of array")
	}
	if _, err := Validate(`"hello"`, true); err == nil {
		t.Fatal("expected error for string instead of array")
	}
}

func TestValidateEmptyList(t *testing.T) {
	if _, err := Validate(`[]`, true); err == nil {
		t.Fatal("expected error for empty list")
	}
}

func TestValidateMissingFields(t *testing.T) {
	cases := []string{
		`[{"api_token":"t","account_id":"i","target_workers":5}]`, // no name
		`[{"name":"a","account_id":"i","target_workers":5}]`,      // no api_token
		`[{"name":"a","api_token":"t","target_workers":5}]`,       // no account_id
	}
	for i, c := range cases {
		if _, err := Validate(c, true); err == nil {
			t.Errorf("case %d: expected error for missing field", i)
		}
	}
}

func TestValidateMissingTargetInCreate(t *testing.T) {
	_, err := Validate(`[{"name":"a","api_token":"t","account_id":"i"}]`, true)
	if err == nil {
		t.Fatal("expected error: target_workers required in create")
	}
}

func TestValidateNegativeTarget(t *testing.T) {
	_, err := Validate(`[{"name":"a","api_token":"t","account_id":"i","target_workers":-1}]`, true)
	if err == nil {
		t.Fatal("expected error: negative target_workers")
	}
}

func TestValidateDuplicateNames(t *testing.T) {
	_, err := Validate(`[
	  {"name":"a","api_token":"t","account_id":"i","target_workers":1},
	  {"name":"a","api_token":"t2","account_id":"i2","target_workers":2}
	]`, true)
	if err == nil {
		t.Fatal("expected error: duplicate account name")
	}
}

func TestValidateNoTargetRequiredForUse(t *testing.T) {
	accs, err := Validate(`[{"name":"a","api_token":"t","account_id":"i"}]`, false)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if len(accs) != 1 {
		t.Fatalf("got %d accounts, want 1", len(accs))
	}
}

func TestValidateZoneIDOptional(t *testing.T) {
	accs, err := Validate(`[{"name":"a","api_token":"t","account_id":"i","zone_id":"z"}]`, false)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if accs[0].ZoneID != "z" {
		t.Errorf("ZoneID = %q, want z", accs[0].ZoneID)
	}
}

func TestValidateDeleteRequiresTargetWorkers(t *testing.T) {
	if _, err := Validate(`[{"name":"a","api_token":"t","account_id":"i"}]`, true); err == nil {
		t.Fatal("expected target_workers to be required for delete")
	}
}

func TestValidateDeleteAcceptsZeroTargetWorkers(t *testing.T) {
	if _, err := Validate(`[{"name":"a","api_token":"t","account_id":"i","target_workers":0}]`, true); err != nil {
		t.Fatalf("zero target should be valid for delete: %v", err)
	}
}

func TestValidateReturnsNoPartialAccountsOnError(t *testing.T) {
	accounts, err := Validate(`[{"name":"a","api_token":"t","account_id":"i"},{"name":"b","api_token":"u"}]`, false)
	if err == nil || accounts != nil {
		t.Fatalf("expected atomic validation failure: accounts=%+v err=%v", accounts, err)
	}
}

func TestValidateRejectsUnknownFieldsAndTrailingData(t *testing.T) {
	if _, err := Validate(`[{"name":"a","api_token":"t","account_id":"i","unexpected":true}]`, false); err == nil {
		t.Fatal("expected unknown field to be rejected")
	}
	if _, err := Validate(`[{"name":"a","api_token":"t","account_id":"i"}] []`, false); err == nil {
		t.Fatal("expected trailing JSON to be rejected")
	}
}
