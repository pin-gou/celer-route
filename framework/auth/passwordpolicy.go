// Package auth provides shared authentication primitives for pg-gateway,
// including the dashboard admin password policy. Centralising the rule here
// keeps the HTTP handler, the CLI reset tool, and any future admin-facing
// surface in lockstep — changing the policy in one place updates them all.
package auth

// GetPasswordPolicyFailures returns the list of policy requirements that the
// given password fails to satisfy. An empty result means the password is
// acceptable. The rules mirror the UI validation in
// ui/lib/utils/validation.ts::getPasswordPolicyFailures so the CLI accepts
// exactly the same passwords the Web UI does.
//
// Rules:
//   - at least 12 characters
//   - at least one uppercase letter
//   - at least one lowercase letter
//   - at least one digit
//   - at least one special (non-alphanumeric) character
func GetPasswordPolicyFailures(password string) []string {
	failures := make([]string, 0, 5)
	hasUppercase := false
	hasLowercase := false
	hasDigit := false
	hasSpecial := false

	for i := 0; i < len(password); i++ {
		char := password[i]
		switch {
		case char >= 'A' && char <= 'Z':
			hasUppercase = true
		case char >= 'a' && char <= 'z':
			hasLowercase = true
		case char >= '0' && char <= '9':
			hasDigit = true
		default:
			hasSpecial = true
		}
	}

	if len(password) < 12 {
		failures = append(failures, "at least 12 characters")
	}
	if !hasUppercase {
		failures = append(failures, "one uppercase letter")
	}
	if !hasLowercase {
		failures = append(failures, "one lowercase letter")
	}
	if !hasDigit {
		failures = append(failures, "one number")
	}
	if !hasSpecial {
		failures = append(failures, "one special character")
	}

	return failures
}
