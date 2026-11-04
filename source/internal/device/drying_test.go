package device

import "testing"

func TestAMSDrying(t *testing.T) {
	for _, tc := range []struct {
		info string
		want bool
	}{{"11401123", true}, {"00000003", false}, {"00000013", true}, {"00000033", true}, {"00000053", true}, {"00000083", false}, {"00000022", false}, {"invalid", false}} {
		raw := map[string]any{"ams": map[string]any{"ams": []any{map[string]any{"info": tc.info}}}}
		if amsDrying(raw) != tc.want {
			t.Fatal(tc)
		}
	}
}
