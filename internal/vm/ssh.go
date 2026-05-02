package vm

import (
	"bytes"
	"fmt"
	"time"

	"github.com/briandowns/spinner"
	"golang.org/x/crypto/ssh"
)

// SSHClient はSSH接続を管理する構造体です
type SSHClient struct {
	client *ssh.Client
}

// WaitForSSH はVMのSSHが起動するまでポーリング（待機）して接続します
func WaitForSSH(port int, user, password string, timeoutMinutes int) (*SSHClient, error) {
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		// 開発用VMなのでホストキーの検証はスキップ (StrictHostKeyChecking=no に相当)
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         2 * time.Second,
	}

	address := fmt.Sprintf("localhost:%d", port)
	
	// スピナー（くるくるアニメーション）の設定
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Prefix = "[Waiting for SSH] "
	s.Start()
	defer s.Stop()

	timeout := time.After(time.Duration(timeoutMinutes) * time.Minute)
	tick := time.Tick(1 * time.Second)

	for {
		select {
		case <-timeout:
			return nil, fmt.Errorf("ssh connection timeout after %d minutes", timeoutMinutes)
		case <-tick:
			client, err := ssh.Dial("tcp", address, config)
			if err == nil {
				s.FinalMSG = "[Waiting for SSH] UP!\n"
				return &SSHClient{client: client}, nil
			}
			// エラーの場合はまだ起動していないのでリトライ
		}
	}
}

// Run はSSH経由でコマンドを実行し、出力を返します
func (s *SSHClient) Run(cmd string) (string, error) {
	session, err := s.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	err = session.Run(cmd)
	if err != nil {
		return "", fmt.Errorf("command execution failed: %w, stderr: %s", err, stderrBuf.String())
	}

	return stdoutBuf.String(), nil
}

// Close はSSH接続を閉じます
func (s *SSHClient) Close() {
	if s.client != nil {
		s.client.Close()
	}
}