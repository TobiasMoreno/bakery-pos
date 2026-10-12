package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	params := PasswordParams{Memory: 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}
	encoded, err := HashPassword("correct-horse", params)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	valid, err := VerifyPassword("correct-horse", encoded)
	if err != nil || !valid {
		t.Fatalf("VerifyPassword() = %v, %v", valid, err)
	}
	valid, err = VerifyPassword("wrong-password", encoded)
	if err != nil {
		t.Fatalf("VerifyPassword(wrong) error = %v", err)
	}
	if valid {
		t.Fatal("wrong password should not verify")
	}
}

func TestHashPasswordRejectsShortCredential(t *testing.T) {
	if _, err := HashPassword("short", DefaultPasswordParams()); err == nil {
		t.Fatal("HashPassword() expected error")
	}
}
