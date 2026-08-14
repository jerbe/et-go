package central

import (
	"errors"
	"testing"
)

func TestMD5Hash(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"123456", "e10adc3949ba59abbe56e057f20f883e"},
		{"", "d41d8cd98f00b204e9800998ecf8427e"},
		{"中文", "a7bac2239fcdcb3a067903d8077c4a07"},
		{"abcdefghijklmnopqrstuvwxyz", "c3fcd3d76192e4007dfb496cca67e13b"},
	}

	for _, tc := range cases {
		if got := MD5Hash(tc.input); got != tc.want {
			t.Fatalf("MD5Hash(%q) = %q, want %q", tc.input, got, tc.want)
		}
		if len(MD5Hash(tc.input)) != 32 {
			t.Fatalf("MD5Hash(%q) length != 32", tc.input)
		}
	}
}

func TestHashAndVerifyArgon2id(t *testing.T) {
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatalf("HashPassword err = %v", err)
	}
	if len(hash) == 0 || hash[:9] != "$argon2id" {
		t.Fatalf("unexpected hash format %q", hash)
	}
	valid, needsUpgrade, err := VerifyPassword("correct horse", hash, PasswordAlgorithmArgon2id)
	if err != nil || !valid || needsUpgrade {
		t.Fatalf("VerifyPassword valid=%v upgrade=%v err=%v", valid, needsUpgrade, err)
	}
	valid, _, err = VerifyPassword("wrong", hash, PasswordAlgorithmArgon2id)
	if err != nil || valid {
		t.Fatalf("wrong password valid=%v err=%v", valid, err)
	}
}

func TestVerifyLegacyMD5RequestsUpgrade(t *testing.T) {
	valid, needsUpgrade, err := VerifyPassword("pass", MD5Hash("pass"), "")
	if err != nil || !valid || !needsUpgrade {
		t.Fatalf("legacy VerifyPassword valid=%v upgrade=%v err=%v", valid, needsUpgrade, err)
	}
}

func TestVerifyPasswordRejectsInvalidAlgorithmAndHash(t *testing.T) {
	if _, _, err := VerifyPassword("pass", "hash", "bcrypt"); !errors.Is(err, ErrPasswordAlgorithmUnsupported) {
		t.Fatalf("unsupported algorithm error = %v", err)
	}
	if _, _, err := VerifyPassword("pass", "", PasswordAlgorithmArgon2id); err != ErrPasswordHashInvalid {
		t.Fatalf("empty hash error = %v", err)
	}
}
