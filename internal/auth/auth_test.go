package auth

import "testing"

func TestTokenValid(t *testing.T) {
	cases := []struct {
		name      string
		expected  string
		presented string
		want      bool
	}{
		{"match", "s3cret", "s3cret", true},
		{"mismatch", "s3cret", "wrong", false},
		{"empty expected rejects", "", "anything", false},
		{"empty presented rejects", "s3cret", "", false},
		{"both empty rejects", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TokenValid(tc.expected, tc.presented); got != tc.want {
				t.Errorf("TokenValid(%q,%q) = %v, want %v", tc.expected, tc.presented, got, tc.want)
			}
		})
	}
}
