package vm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// CalculateStepHash は、直前のハッシュ値とステップの情報から新しいハッシュ（12文字）を計算します
func CalculateStepHash(prevHash string, stepType string, command string) string {
	// 直前の状態と、今回のコマンドを連結した文字列を作る
	data := fmt.Sprintf("%s|%s|%s", prevHash, stepType, command)
	
	// SHA256でハッシュ化
	hash := sha256.Sum256([]byte(data))
	
	// 扱いやすいように16進数文字列にし、Dockerのように先頭12文字だけを使う
	return hex.EncodeToString(hash[:])[:12]
}