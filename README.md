# kylnz 🚀

**kylnz** は、QEMU と QEMU Guest Agent (QGA) を活用した、Docker ライクな超高速ローカル VM イメージビルダーです。

QGA と virtio-serial を通してゲスト内に直接コマンドを流し込み、各コマンド実行ごとに `qcow2` スナップショットを自動生成します。これにより、一度成功したステップは瞬時にキャッシュから復元でき、インフラのトライ＆エラーを非常に速く回せます。

## ✨ 主な機能 (Features)

- **瞬時のキャッシュ再開 (Layer Caching):** コマンド実行ごとに `qcow2` スナップショットを作成。失敗しても途中から一瞬でリカバリできます。
- **SSH・ネットワーク非依存 (No SSH Required):** QGA を利用するため、SSH やネットワーク設定に依存せず安定してコマンドを実行できます。
- **宣言的な設定ファイル (Kylnzfile):** YAML で記述するシンプルな設定ファイルにステップを列挙するだけでビルドできます。

## 📦 動作環境 (Prerequisites)

- **Go:** 1.20 以上
- **QEMU:** `qemu-system-x86_64`, `qemu-img` など
- **Linux + KVM:** `-enable-kvm` が利用できる環境を推奨

## 🛠 事前準備 (Base Image Setup)

kylnz を使うには、ベースイメージ（`base.qcow2` など）に `qemu-guest-agent` と `acpid` をインストールし、自動起動を有効化しておく必要があります。

例：Alpine Linux ベースイメージの準備

```bash
# ISO からインストール後、ゲスト内で実行
apk add qemu-guest-agent acpid
rc-update add qemu-guest-agent default
rc-update add acpid default
poweroff
```

## 🚀 インストール (Installation)

リポジトリをクローンしてビルドします。

```bash
git clone https://github.com/yourusername/kylnz.git
cd kylnz
go build -o kylnz
```

## 📖 使い方 (Usage)

1) プロジェクトディレクトリに `Kylnzfile` という YAML ファイルを作成します。例：

```yaml
vmid: "my-custom-vm"
vmname: "My Custom Application VM"
base_image: "/path/to/your/base.qcow2"
memory: 1024
output_dir: "./.kylnz/layers"

steps:
  - type: "run"
    command: "apk update"

  - type: "run"
    command: "apk add vim curl nginx"

  - type: "run"
    command: "echo '<h1>Hello Kylnz!</h1>' > /var/www/localhost/htdocs/index.html"

  - type: "run"
    command: "adduser -D kylnzuser"

  - type: "run"
    command: "echo 'kylnzuser:password123' | chpasswd"
```

2) ビルドの実行

```bash
./kylnz build
```

すでに完了しているステップは自動検知され、キャッシュから即座にスキップされます。生成されたレイヤーのディスクイメージは `output_dir`（デフォルト `./.kylnz/layers/`）に保存されます。

## 🏗 アーキテクチャ (Architecture)

- Start: `base.qcow2` を元に QEMU を起動し、QGA 用の UNIX ドメインソケットを開きます。
- Connect: OS 起動後、ゲスト内の `qemu-guest-agent` の応答を待ちます。
- Snapshot: コマンド実行直前に QEMU Monitor 経由でディスクを同期し、スナップショットを作成します。
- Execute: QGA の `guest-exec` を JSON 形式で発行してコマンドを実行し、標準出力／標準エラーを受け取ります。

## 参考 (Notes)

- ベースイメージに QGA が正しくインストールされ自動起動していることを必ず確認してください。
- 高速化の要点はディスクスナップショットと差分イメージの活用です。

---
