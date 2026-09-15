package client

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"

	"go.uber.org/zap"
)

func (LEZ *LE_EZVIZ_Client) LoadFeatureCode(path string) string {
	if b, err := os.ReadFile(path); err == nil && len(b) == 32 {
		return LEZ.applyFeatureCode(string(b))
	}
	code := hex.EncodeToString(GenerateFeatureCode())
	if err := os.WriteFile(path, []byte(code), 0o644); err != nil {
		log.Error("featurecode file", zap.Error(err))
	}
	return LEZ.applyFeatureCode(code)
}

func (LEZ *LE_EZVIZ_Client) applyFeatureCode(code string) string {
	LEZ.FeatureCode = code
	LEZ.Headers["featureCode"] = []string{code}
	return code
}

func GetMd5(text string) string {
	h := md5.Sum([]byte(text))
	return hex.EncodeToString(h[:])
}

func GenerateFeatureCode() []byte {
	b := make([]byte, 16)
	_, _ = io.ReadFull(rand.Reader, b)
	h := md5.Sum(b)
	return h[:]
}
