package crypton

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/argon2"
)

var (
	ErrInvalidSignature = errors.New("invalid signature")
	ErrInvalidHash      = errors.New("invalid password hash format")
)

// pepper é um segredo global aplicado antes do hash — definido via SetPepper no bootstrap.
// Nunca armazenado no banco; protege contra vazamento de hashes mesmo com acesso ao DB.
var pepper string

// SetPepper define o pepper global usado em HashPassword e CheckPassword.
// Deve ser chamado uma única vez no bootstrap, antes de qualquer operação de senha.
func SetPepper(p string) { pepper = p }

// Parâmetros Argon2id — baseados nas recomendações OWASP (mínimo seguro).
const (
	argonMemory      = 64 * 1024 // 64 MB
	argonIterations  = 3
	argonParallelism = 4
	argonSaltLen     = 16
	argonKeyLen      = 32
)

// HashPassword gera um hash Argon2id no formato PHC:
// $argon2id$v=19$m=65536,t=3,p=4$<salt-base64>$<hash-base64>
// O pepper (definido via SetPepper) é aplicado antes da derivação da chave.
func HashPassword(password string) (string, error) {
	salt, err := RandomBytes(argonSaltLen)
	if err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password+pepper), salt, argonIterations, argonMemory, argonParallelism, argonKeyLen)
	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argonMemory, argonIterations, argonParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
	return encoded, nil
}

// CheckPassword verifica plain contra um hash Argon2id no formato PHC.
// A comparação é feita em tempo-constante para evitar timing attacks.
func CheckPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	// formato: ["", "argon2id", "v=19", "m=...,t=...,p=...", "<salt>", "<hash>"]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}

	var mem, iter uint32
	var par uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &iter, &par); err != nil {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	storedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}

	candidate := argon2.IDKey([]byte(password+pepper), salt, iter, mem, par, uint32(len(storedHash)))
	return subtle.ConstantTimeCompare(candidate, storedHash) == 1
}

func Sign(payload, secret string) (string, error) {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	encoded := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return encoded + "." + sig, nil
}

func Verify(token, secret string) (string, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return "", ErrInvalidSignature
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", ErrInvalidSignature
	}

	// Computa o MAC esperado diretamente e compara em tempo-constante,
	// evitando timing attacks na verificação da assinatura.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedSig := mac.Sum(nil)

	submittedSig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(expectedSig, submittedSig) {
		return "", ErrInvalidSignature
	}

	return string(payload), nil
}

func RandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return b, err
}

// encryptionKey é derivada via SHA-256 da chave definida em SetEncryptionKey.
var encryptionKey []byte

// SetEncryptionKey deriva uma chave AES-256 a partir de key (qualquer tamanho)
// e a armazena para uso em Encrypt/Decrypt. Chamado uma vez no bootstrap.
func SetEncryptionKey(key string) {
	h := sha256.Sum256([]byte(key))
	encryptionKey = h[:]
}

const encPrefix = "$enc$"

// Encrypt cifra plaintext com AES-256-GCM e devolve "$enc$<base64>".
// Se o valor já tiver o prefixo, retorna sem modificar (idempotente).
func Encrypt(plaintext string) (string, error) {
	if strings.HasPrefix(plaintext, encPrefix) {
		return plaintext, nil
	}
	if len(encryptionKey) == 0 {
		return "", errors.New("crypton: chave de criptografia não definida — chame SetEncryptionKey no bootstrap")
	}
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(ct), nil
}

// Decrypt decifra um valor produzido por Encrypt.
// Se o valor não tiver o prefixo "$enc$", é devolvido sem alteração.
func Decrypt(ciphertext string) (string, error) {
	if !strings.HasPrefix(ciphertext, encPrefix) {
		return ciphertext, nil
	}
	if len(encryptionKey) == 0 {
		return "", errors.New("crypton: chave de criptografia não definida — chame SetEncryptionKey no bootstrap")
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(ciphertext, encPrefix))
	if err != nil {
		return "", fmt.Errorf("crypton: decrypt base64: %w", err)
	}
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("crypton: ciphertext muito curto")
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("crypton: decrypt: %w", err)
	}
	return string(plain), nil
}
