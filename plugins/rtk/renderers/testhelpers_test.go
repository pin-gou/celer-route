package renderers

import "encoding/json"

// jsonMarshalForTest is a tiny shim that wraps encoding/json so test
// fixtures can produce JSON strings without importing the package
// directly. Kept in a separate file to avoid polluting the production
// source with test helpers.
func jsonMarshalForTest(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}