package version

import "testing"

func TestValid(t *testing.T) {
	for v, want := range map[string]bool{
		"1.4.0":                 true,
		"0.1.0":                 true,
		"1.5.0-rc.1":            true,
		"v1.4.0":                false,
		"dev":                   false,
		"dev-v0.1.0-3-gabc1234": false,
		"dev-540980c":           false,
		"1.4":                   false,
		"1.4.0+meta":            false,
		"":                      false,
	} {
		if got := Valid(v); got != want {
			t.Errorf("Valid(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.2.0", "0.1.0", true},
		{"0.10.0", "0.9.0", true},
		{"1.0.0", "1.0.0-rc.1", true},
		{"1.0.0-rc.1", "1.0.0", false},
		{"0.1.0", "0.1.0", false},
		{"0.1.0", "0.2.0", false},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
