package handlers

import (
	"reflect"
	"strings"
	"testing"
)

func TestGetPasswordPolicyFailures(t *testing.T) {
	tests := []struct {
		name     string
		password string
		want     []string
	}{
		{
			name:     "valid password",
			password: "StrongPass1!",
			want:     []string{},
		},
		{
			name:     "missing all requirements",
			password: "",
			want: []string{
				"at least 12 characters",
				"one uppercase letter",
				"one lowercase letter",
				"one number",
				"one special character",
			},
		},
		{
			name:     "missing character classes",
			password: "weakpassword",
			want: []string{
				"one uppercase letter",
				"one number",
				"one special character",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getPasswordPolicyFailures(tt.password)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("getPasswordPolicyFailures() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateApplicationLogSettings(t *testing.T) {
	tests := []struct {
		name          string
		logLevel      string
		logOutputStyle string
		wantErr       bool
		errSubstr     string
	}{
		{name: "both empty (follow boot args)", logLevel: "", logOutputStyle: ""},
		{name: "valid level only", logLevel: "debug", logOutputStyle: ""},
		{name: "valid style only", logLevel: "", logOutputStyle: "pretty"},
		{name: "valid both", logLevel: "warn", logOutputStyle: "json"},
		{name: "error level valid", logLevel: "error", logOutputStyle: "json"},
		{name: "invalid level rejected", logLevel: "verbose", logOutputStyle: "", wantErr: true, errSubstr: "log_level"},
		{name: "invalid style rejected", logLevel: "info", logOutputStyle: "text", wantErr: true, errSubstr: "log_output_style"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateApplicationLogSettings(tt.logLevel, tt.logOutputStyle)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("validateApplicationLogSettings(%q, %q) = nil, want error", tt.logLevel, tt.logOutputStyle)
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Fatalf("validateApplicationLogSettings(%q, %q) error = %q, want substr %q", tt.logLevel, tt.logOutputStyle, err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateApplicationLogSettings(%q, %q) = %v, want nil", tt.logLevel, tt.logOutputStyle, err)
			}
		})
	}
}
