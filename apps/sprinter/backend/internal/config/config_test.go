package config

import (
	"slices"
	"testing"
)

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{name: "no database", config: Config{}, wantErr: true},
		{name: "database alone is enough", config: Config{DatabaseURL: "postgres://x"}},
		{
			name:    "a token without a guild cannot register a command",
			config:  Config{DatabaseURL: "postgres://x", DiscordToken: "t"},
			wantErr: true,
		},
		{
			name:   "token and guild together",
			config: Config{DatabaseURL: "postgres://x", DiscordToken: "t", DiscordGuildID: "1"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := testCase.config.Validate(); (err != nil) != testCase.wantErr {
				t.Fatalf("Validate() = %v, wantErr %t", err, testCase.wantErr)
			}
		})
	}
}

func TestAllowedOrigins(t *testing.T) {
	config := Config{
		FrontendURL: "http://localhost:5178/",
		CORSOrigins: []string{"http://127.0.0.1:5178", "http://localhost:5178", " "},
	}
	want := []string{"http://localhost:5178", "http://127.0.0.1:5178"}
	if got := config.AllowedOrigins(); !slices.Equal(got, want) {
		t.Fatalf("AllowedOrigins() = %q, want %q", got, want)
	}
	// Credentials plus an empty list would make Fiber fall back to the
	// wildcard and panic, so the default always stands in.
	if got := (Config{}).AllowedOrigins(); !slices.Equal(got, []string{DefaultFrontendURL}) {
		t.Fatalf("AllowedOrigins() = %q", got)
	}
}
