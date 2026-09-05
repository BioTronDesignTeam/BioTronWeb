package auth

import "testing"

func TestSafeReturn(t *testing.T) {
	h := &Handler{Cfg: Config{
		FrontendURL:    "http://localhost:5173",
		AllowedOrigins: []string{"http://localhost:5173", "http://localhost:5174"},
	}}
	cases := []struct {
		in, want string
	}{
		{"", "http://localhost:5173"},
		{"http://localhost:5174", "http://localhost:5174"},
		{"http://localhost:5174/dashboard", "http://localhost:5174"},
		{"https://evil.example", "http://localhost:5173"},
		{"javascript:alert(1)", "http://localhost:5173"},
		{"/relative", "http://localhost:5173"},
	}
	for _, tc := range cases {
		if got := h.safeReturn(tc.in); got != tc.want {
			t.Errorf("safeReturn(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
