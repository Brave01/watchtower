// Package auth 提供 log-monitor 与 dashboard(lvneng) 两个独立进程共用的
// JWT 签发/校验能力。两边通过同一个 AUTH_JWT_SECRET 签名，
// 使得在同一主机名下登录一次即可在两个端口间免登录跳转（SSO）。
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrMalformedToken   = errors.New("auth: malformed token")
	ErrInvalidSignature = errors.New("auth: invalid signature")
	ErrExpiredToken     = errors.New("auth: token expired")
)

// Claims 是签发到 JWT payload 里的内容。
type Claims struct {
	Subject   string `json:"sub"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

func b64Encode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func b64Decode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// Sign 用 secret 签发一个 HS256 JWT，有效期 ttl。
func Sign(secret []byte, subject string, ttl time.Duration) (string, error) {
	now := time.Now()
	header, err := json.Marshal(jwtHeader{Alg: "HS256", Typ: "JWT"})
	if err != nil {
		return "", err
	}
	claims, err := json.Marshal(Claims{
		Subject:   subject,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(ttl).Unix(),
	})
	if err != nil {
		return "", err
	}

	signingInput := b64Encode(header) + "." + b64Encode(claims)
	sig := sign(secret, signingInput)
	return signingInput + "." + b64Encode(sig), nil
}

// Verify 校验 token 的签名与有效期，返回其中的 Claims。
func Verify(secret []byte, token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrMalformedToken
	}
	signingInput := parts[0] + "." + parts[1]
	gotSig, err := b64Decode(parts[2])
	if err != nil {
		return nil, ErrMalformedToken
	}
	wantSig := sign(secret, signingInput)
	if subtle.ConstantTimeCompare(gotSig, wantSig) != 1 {
		return nil, ErrInvalidSignature
	}

	claimsRaw, err := b64Decode(parts[1])
	if err != nil {
		return nil, ErrMalformedToken
	}
	var claims Claims
	if err := json.Unmarshal(claimsRaw, &claims); err != nil {
		return nil, ErrMalformedToken
	}
	if time.Now().Unix() > claims.ExpiresAt {
		return nil, ErrExpiredToken
	}
	return &claims, nil
}

func sign(secret []byte, input string) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(input))
	return mac.Sum(nil)
}
