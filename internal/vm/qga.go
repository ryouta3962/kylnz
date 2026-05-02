package vm

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

type QGAClient struct {
	conn    net.Conn
	scanner *bufio.Scanner
}

// QGAソケットに接続し、VM内のエージェントが起動するのを待つ
func ConnectQGA(sockPath string, timeoutSec int) (*QGAClient, error) {
	var conn net.Conn
	var err error
	
	// ソケットファイルができるまで待機
	for i := 0; i < timeoutSec*2; i++ {
		conn, err = net.Dial("unix", sockPath)
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to connect to QGA socket: %w", err)
	}

	client := &QGAClient{
		conn:    conn,
		scanner: bufio.NewScanner(conn),
	}

	// エージェントが応答するまで(OSが起動するまで) ping を送り続ける
	fmt.Print("Waiting for OS to boot and QGA to start ")
	for i := 0; i < timeoutSec; i++ {
		fmt.Print(".")
		_, err := client.sendCommand("guest-ping", nil)
		if err == nil {
			fmt.Println(" Connected!")
			return client, nil
		}
		time.Sleep(1 * time.Second)
	}
	
	client.Close()
	return nil, fmt.Errorf(" QGA timeout")
}

func (c *QGAClient) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// 任意のコマンドをVM内で実行する
func (c *QGAClient) RunCommand(command string) (string, error) {
	// 1. コマンドの実行リクエスト ( /bin/sh -c "コマンド" として実行 )
	args := map[string]interface{}{
		"path":           "/bin/sh",
		"arg":            []string{"-c", command},
		"capture-output": true,
	}
	
	resp, err := c.sendCommand("guest-exec", args)
	if err != nil {
		return "", fmt.Errorf("exec request failed: %w", err)
	}

	// PIDを取得
	retMap, ok := resp["return"].(map[string]interface{})
	if !ok || retMap["pid"] == nil {
		return "", fmt.Errorf("invalid response, no pid returned")
	}
	pid := int(retMap["pid"].(float64))

	// 2. コマンドの終了をポーリングして待つ
	for {
		statusResp, err := c.sendCommand("guest-exec-status", map[string]interface{}{"pid": pid})
		if err != nil {
			return "", err
		}

		statusRet := statusResp["return"].(map[string]interface{})
		if exited, ok := statusRet["exited"].(bool); ok && exited {
			// 終了したらBase64エンコードされた出力をデコードする
			var output string
			if outData, ok := statusRet["out-data"].(string); ok {
				decoded, _ := base64.StdEncoding.DecodeString(outData)
				output += string(decoded)
			}
			if errData, ok := statusRet["err-data"].(string); ok {
				decoded, _ := base64.StdEncoding.DecodeString(errData)
				output += string(decoded)
			}

			// 終了コードの確認
			if exitCode, ok := statusRet["exitcode"].(float64); ok && exitCode != 0 {
				return output, fmt.Errorf("command exited with code %d: %s", int(exitCode), output)
			}
			return output, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// QGAにJSONを送信してレスポンスを受け取る共通処理
func (c *QGAClient) sendCommand(execute string, args map[string]interface{}) (map[string]interface{}, error) {
	req := map[string]interface{}{"execute": execute}
	if args != nil {
		req["arguments"] = args
	}

	reqBytes, _ := json.Marshal(req)
	_, err := fmt.Fprintf(c.conn, "%s\n", reqBytes)
	if err != nil {
		return nil, err
	}

	if !c.scanner.Scan() {
		return nil, fmt.Errorf("failed to read response")
	}

	var resp map[string]interface{}
	err = json.Unmarshal(c.scanner.Bytes(), &resp)
	if err != nil {
		return nil, err
	}
	
	if errorResp, hasError := resp["error"]; hasError {
		return nil, fmt.Errorf("QGA error: %v", errorResp)
	}

	return resp, nil
}
