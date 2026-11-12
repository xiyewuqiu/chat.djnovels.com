package services

import (
	"strings"
	"testing"

	utils "github.com/juggleim/jugglechat-server/commons/tools"
)

func TestHashPasswordProducesVerifiableBcrypt(t *testing.T) {
	hash, err := HashPassword("secret123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(hash, passwordHashPrefix) {
		t.Fatalf("hash %q lost its prefix", hash)
	}
	if len(hash) > 255 {
		t.Fatalf("hash length %d exceeds login_pass column", len(hash))
	}
	if !VerifyPassword(hash, "secret123") {
		t.Fatal("correct password rejected")
	}
	if VerifyPassword(hash, "secret124") {
		t.Fatal("wrong password accepted")
	}
	if PasswordNeedsRehash(hash) {
		t.Fatal("bcrypt hash must not ask for a rehash")
	}
}

// 存量 SHA1 摘要必须继续可用，并在登录后被标记为需要升级。
func TestLegacySha1StillVerifiesAndNeedsRehash(t *testing.T) {
	legacy := utils.SHA1("oldpass")
	if !VerifyPassword(legacy, "oldpass") {
		t.Fatal("legacy sha1 rejected")
	}
	if VerifyPassword(legacy, "oldpass2") {
		t.Fatal("legacy sha1 accepted a wrong password")
	}
	if !PasswordNeedsRehash(legacy) {
		t.Fatal("legacy sha1 must be upgraded")
	}
}

func TestVerifyPasswordRejectsEmptyInputs(t *testing.T) {
	if VerifyPassword("", "x") {
		t.Fatal("empty stored hash accepted")
	}
	if VerifyPassword("x", "") {
		t.Fatal("empty password accepted")
	}
	if PasswordNeedsRehash("") {
		t.Fatal("empty hash must not trigger an upgrade write")
	}
}

// bcrypt 只看前 72 字节；超长口令按截断处理，保证不报错、行为可预期。
func TestHashPasswordTruncatesBeyondBcryptLimit(t *testing.T) {
	long := strings.Repeat("a", 100)
	hash, err := HashPassword(long)
	if err != nil {
		t.Fatalf("hash long password: %v", err)
	}
	if !VerifyPassword(hash, long) {
		t.Fatal("long password rejected")
	}
	if !VerifyPassword(hash, long[:bcryptMaxBytes]) {
		t.Fatal("truncated prefix must verify")
	}
	if VerifyPassword(hash, long[:bcryptMaxBytes-1]+"b") {
		t.Fatal("different prefix accepted")
	}
}
