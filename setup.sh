#!/bin/bash
set -e

WORKSPACE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BINARY_PATH="${WORKSPACE_DIR}/wheelpad-go"
SERVICE_DIR="${HOME}/.config/systemd/user"
SERVICE_FILE="${SERVICE_DIR}/wheelpad.service"

echo "=== Lets Note Wheelpad Scroll Daemon (Go) ユーザーセットアップ ==="

# 1. Check if the binary is built
if [ ! -f "$BINARY_PATH" ]; then
    echo "バイナリが見つかりません。ビルドを実行します..."
    (cd "$WORKSPACE_DIR" && go build -o wheelpad-go)
fi

# 2. Ensure systemd user configuration directory exists
mkdir -p "$SERVICE_DIR"

# 3. Create the systemd --user service file
echo "systemd --user サービスファイルを作成しています..."
cat << EOF > "$SERVICE_FILE"
[Unit]
Description=Lets Note Wheelpad Scroll Daemon
After=graphical-session.target

[Service]
Type=simple
ExecStart=${BINARY_PATH}
Restart=always
RestartSec=2

[Install]
WantedBy=default.target
EOF

# 4. Create a temporary udev rules file
UDEV_RULE_TEMP=$(mktemp)
cat << EOF > "$UDEV_RULE_TEMP"
# /etc/udev/rules.d/99-wheelpad.rules
# Enable access to /dev/uinput for the currently logged-in user
KERNEL=="uinput", TAG+="uaccess"

# Enable access to the physical touchpad for the currently logged-in user
ATTRS{name}=="Synaptics TM3562-003", TAG+="uaccess"
EOF

echo "udev ルールを配置するために sudo パスワードの入力が必要です..."
sudo mv "$UDEV_RULE_TEMP" /etc/udev/rules.d/99-wheelpad.rules
sudo chown root:root /etc/udev/rules.d/99-wheelpad.rules
sudo chmod 644 /etc/udev/rules.d/99-wheelpad.rules

echo "udev ルールをリロードしています..."
sudo udevadm control --reload-rules
sudo udevadm trigger

# 5. Reload systemd user daemon and enable the service
echo "systemd ユーザーサービスをリロードし、自動起動を有効化しています..."
systemctl --user daemon-reload
systemctl --user enable --now wheelpad.service

echo "=== セットアップが完了しました！ ==="
echo "サービス状態の確認: systemctl --user status wheelpad.service"
echo "サービスログの確認: journalctl --user -u wheelpad.service -f"
echo "※ もしデバイスの権限が即座に反映されない場合は、一度USBの再挿入やログアウト・再ログインをお試しください。"
