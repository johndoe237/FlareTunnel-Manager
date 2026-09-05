package business

import "encoding/json"

// jsonMarshal is a tiny indirection so the business package does not import
// encoding/json directly at call sites.
func jsonMarshal(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}
