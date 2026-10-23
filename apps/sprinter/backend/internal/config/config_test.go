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
			name:   "disabled ignores the Discord credentials",
			config: Config{DatabaseURL: "postgres://x", DiscordToken: "t"},
		},
		{
			name:    "enabled without a token cannot open a session",
			config:  Config{DatabaseURL: "postgres://x", Discord: true, DiscordGuildID: "1"},
			wantErr: true,
		},
		{
			name:    "a token without a guild cannot register a command",
			config:  Config{DatabaseURL: "postgres://x", Discord: true, DiscordToken: "t"},
			wantErr: true,
		},
		{
			name:   "token and guild together",
			config: Config{DatabaseURL: "postgres://x", Discord: true, DiscordToken: "t", DiscordGuildID: "1"},
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

func TestLoadReadsTheDiscordFlag(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("DISCORD_TOKEN", "t")
	t.Setenv("DISCORD_GUILD_ID", "1")
	cases := []struct {
		value   string
		want    bool
		wantErr bool
	}{
		{value: "", want: false},
		{value: "false", want: false},
		{value: "0", want: false},
		{value: "true", want: true},
		{value: " TRUE ", want: true},
		{value: "1", want: true},
		{value: "ture", wantErr: true},
		{value: "yes", wantErr: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.value, func(t *testing.T) {
			t.Setenv("SPRINTER_DISCORD_ENABLED", testCase.value)
			config := Load()
			err := config.Validate()
			if (err != nil) != testCase.wantErr {
				t.Fatalf("Validate() = %v, wantErr %t", err, testCase.wantErr)
			}
			if !testCase.wantErr && config.DiscordEnabled() != testCase.want {
				t.Fatalf("DiscordEnabled() = %t, want %t", config.DiscordEnabled(), testCase.want)
			}
		})
	}
}

// Production runs with the flag off and no Discord variables at all, so the
// bare minimum must load and validate.
func TestLoadWithoutDiscordNeedsNoCredentials(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPRINTER_DISCORD_ENABLED", "")
	t.Setenv("DISCORD_TOKEN", "")
	t.Setenv("DISCORD_GUILD_ID", "")
	config := Load()
	if err := config.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
	if config.DiscordEnabled() {
		t.Fatal("Discord is on by default")
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
