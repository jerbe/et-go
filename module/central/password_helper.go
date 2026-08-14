package central

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	// PasswordAlgorithmMD5 是历史账号密码算法，仅用于一次性迁移旧账号。
	PasswordAlgorithmMD5 = "md5"
	// PasswordAlgorithmArgon2id 是新账号和旧账号升级后的密码算法。
	PasswordAlgorithmArgon2id = "argon2id"

	argon2Memory      uint32 = 64 * 1024
	argon2Iterations  uint32 = 3
	argon2Parallelism uint8  = 2
	argon2SaltLength         = 16
	argon2KeyLength          = 32
)

// MD5Hash 计算历史兼容格式的 MD5 小写十六进制。
//
// 新账号不得再使用此函数保存密码；它只用于读取并升级已有旧账号。
func MD5Hash(input string) string {
	sum := md5.Sum([]byte(input))
	return hex.EncodeToString(sum[:])
}

// HashPassword 生成 Argon2id 密码哈希。
func HashPassword(password string) (string, error) {
	salt := make([]byte, argon2SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("%w: generate salt: %v", ErrPasswordHashInvalid, err)
	}
	digest := argon2.IDKey(
		[]byte(password),
		salt,
		argon2Iterations,
		argon2Memory,
		argon2Parallelism,
		argon2KeyLength,
	)
	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argon2Memory,
		argon2Iterations,
		argon2Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(digest),
	), nil
}

// VerifyPassword 校验密码，并返回“是否需要从历史算法升级”标志。
func VerifyPassword(password, storedHash, algorithm string) (valid bool, needsUpgrade bool, err error) {
	if storedHash == "" {
		return false, false, ErrPasswordHashInvalid
	}
	algorithm = strings.ToLower(strings.TrimSpace(algorithm))
	if algorithm == "" {
		if strings.HasPrefix(storedHash, "$argon2id$") {
			algorithm = PasswordAlgorithmArgon2id
		} else {
			algorithm = PasswordAlgorithmMD5
		}
	}

	switch algorithm {
	case PasswordAlgorithmMD5:
		if len(storedHash) != 32 {
			return false, false, ErrPasswordHashInvalid
		}
		expected := MD5Hash(password)
		valid := subtle.ConstantTimeCompare([]byte(strings.ToLower(storedHash)), []byte(expected)) == 1
		return valid, true, nil
	case PasswordAlgorithmArgon2id:
		valid, err := verifyArgon2id(password, storedHash)
		return valid, false, err
	default:
		return false, false, fmt.Errorf("%w: %s", ErrPasswordAlgorithmUnsupported, algorithm)
	}
}

func verifyArgon2id(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false, ErrPasswordHashInvalid
	}
	parameters := make(map[string]uint64, 3)
	for _, item := range strings.Split(parts[3], ",") {
		keyValue := strings.SplitN(item, "=", 2)
		if len(keyValue) != 2 {
			return false, ErrPasswordHashInvalid
		}
		value, err := strconv.ParseUint(keyValue[1], 10, 32)
		if err != nil || value == 0 {
			return false, ErrPasswordHashInvalid
		}
		parameters[keyValue[0]] = value
	}
	memory, okMemory := parameters["m"]
	iterations, okIterations := parameters["t"]
	parallelism, okParallelism := parameters["p"]
	if !okMemory || !okIterations || !okParallelism ||
		memory > uint64(^uint32(0)) || iterations > uint64(^uint32(0)) ||
		parallelism > uint64(^uint8(0)) ||
		memory > 1024*1024 || iterations > 100 || parallelism > 64 {
		return false, ErrPasswordHashInvalid
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return false, ErrPasswordHashInvalid
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) == 0 {
		return false, ErrPasswordHashInvalid
	}
	actual := argon2.IDKey(
		[]byte(password),
		salt,
		uint32(iterations),
		uint32(memory),
		uint8(parallelism),
		uint32(len(expected)),
	)
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}
