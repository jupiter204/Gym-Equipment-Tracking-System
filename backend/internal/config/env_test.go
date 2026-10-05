package config

import (
	"os"
	"testing"
)

func TestIsProduction(t *testing.T) {
	orig := os.Getenv("APP_ENV")
	defer os.Setenv("APP_ENV", orig)

	tests := []struct {
		envValue string
		expected bool
	}{
		{"production", true},
		{"Production", true},
		{" PRODUCTION ", true},
		{"prod", true},
		{"PROD", true},
		{" prod ", true},
		{"", false},
		{"development", false},
		{"staging", false},
		{"test", false},
	}

	for _, tt := range tests {
		t.Run(tt.envValue, func(t *testing.T) {
			os.Setenv("APP_ENV", tt.envValue)
			actual := IsProduction()
			if actual != tt.expected {
				t.Errorf("IsProduction() with APP_ENV=%q: expected %v, got %v", tt.envValue, tt.expected, actual)
			}
		})
	}
}
