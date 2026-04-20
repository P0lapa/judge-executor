package languages

import (
	"slices"
	"testing"
)

func TestGetConfigUsesManagedJudgeImages(t *testing.T) {
	tests := []struct {
		lang      string
		wantImage string
	}{
		{lang: "cpp", wantImage: "judge-cpp:latest"},
		{lang: "java", wantImage: "judge-java:latest"},
		{lang: "python", wantImage: "judge-python:3.10"},
		{lang: "kotlin", wantImage: "judge-kotlin:17"},
	}

	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			cfg, err := GetConfig(tt.lang)
			if err != nil {
				t.Fatalf("GetConfig(%q) error = %v", tt.lang, err)
			}
			if cfg.Image != tt.wantImage {
				t.Fatalf("GetConfig(%q).Image = %q, want %q", tt.lang, cfg.Image, tt.wantImage)
			}
		})
	}
}

func TestGetConfigAddsJvmSafetyFlags(t *testing.T) {
	tests := []struct {
		lang      string
		wantFlags []string
	}{
		{
			lang: "java",
			wantFlags: []string{
				"-XX:ReservedCodeCacheSize=64m",
				"-Xms32m",
				"-Xmx128m",
				"-XX:+UseSerialGC",
			},
		},
		{
			lang: "kotlin",
			wantFlags: []string{
				"-XX:ReservedCodeCacheSize=64m",
				"-Xms32m",
				"-Xmx128m",
				"-XX:+UseSerialGC",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			cfg, err := GetConfig(tt.lang)
			if err != nil {
				t.Fatalf("GetConfig(%q) error = %v", tt.lang, err)
			}
			for _, flag := range tt.wantFlags {
				if !slices.Contains(cfg.RunCmd, flag) {
					t.Fatalf("GetConfig(%q).RunCmd = %v, missing %q", tt.lang, cfg.RunCmd, flag)
				}
			}
		})
	}
}
