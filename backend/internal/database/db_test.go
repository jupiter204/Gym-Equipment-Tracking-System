package database

import (
	"testing"
)

func TestValidateDBPassword(t *testing.T) {
	tests := []struct {
		name    string
		pass    string
		wantErr bool
	}{
		{name: "empty password", pass: "", wantErr: true},
		{name: "default postgres", pass: "postgres", wantErr: true},
		{name: "too short (< 8 chars)", pass: "short1", wantErr: true},
		{name: "contains change_this lowercase", pass: "change_this_to_a_secure_password", wantErr: true},
		{name: "contains CHANGE_THIS uppercase", pass: "CHANGE_THIS_x12345", wantErr: true},
		{name: "strong password", pass: "Str0ngDbPass99", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDBPassword(tt.pass)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDBPassword(%q) error = %v, wantErr = %v", tt.pass, err, tt.wantErr)
			}
		})
	}
}
