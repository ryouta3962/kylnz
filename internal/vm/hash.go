package vm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// CalculateStepHash は、直前のハッシュ値とステップの情報から新しいハッシュ（12文字）を計算します
func CalculateStepHash(prevHash string, stepType string, command string, src string, dest string) (string, error) {
	hash := sha256.New()

	// まず、直前の状態とステップのタイプをハッシュに書き込む
	hash.Write([]byte(fmt.Sprintf("%s|%s|", prevHash, stepType)))

	if stepType == "copy" {
		// dest（送り先パス）をハッシュに書き込む
		hash.Write([]byte(fmt.Sprintf("%s|", dest)))

		// コピー元のファイルを読み込みモードで開く
		f, err := os.Open(src)
		if err != nil {
			return "", fmt.Errorf("failed to open source file '%s' for hashing: %w", src, err)
		}
		defer f.Close()

		// ファイルの中身をストリームで直接ハッシュに流し込む（メモリ効率が良い）
		if _, err := io.Copy(hash, f); err != nil {
			return "", fmt.Errorf("failed to hash source file '%s': %w", src, err)
		}
	} else {
		// "run" などの既存ステップ用
		hash.Write([]byte(command))
	}

	// 扱いやすいように16進数文字列にし、先頭12文字だけを使う
	return hex.EncodeToString(hash.Sum(nil))[:12], nil
}