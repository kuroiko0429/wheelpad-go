# wheelpad-go

Panasonic Let's Noteの「ホイールパッド」（タッチパッドの縁を円を描くようになぞるとスクロールできる機能）を、Wayland環境でも使えるようにするデーモン。Go製。

[wheel-test](https://github.com/kuroiko0429/let-s-note-wheelpad)（Python版）のGo書き直し。

> [!NOTE]
> **動作確認済み環境**:
> - **OS**: CachyOS
> - **WM/Compositor**: Hyprland
> - **Device**: Panasonic Let's Note SV1

## Python版との違い

| | Python版 | Go版 |
| :--- | :--- | :--- |
| 実行方式 | `sudo python wheelpad.py` | 単一バイナリ |
| 仮想デバイス生成 | `evdev.UInput` | `/dev/uinput`への生ioctl呼び出しを自前実装 |
| 慣性スクロール | `threading.Event` + スレッド | `context.CancelFunc` + goroutine |
| 権限 | ユーザーを`input`/`uinput`グループに追加 | udevルールの`TAG+="uaccess"`（グループ変更不要） |
| サービス | systemd system service（root実行） | systemd **user** service |
| 依存 | `pip install evdev` | ビルド済みバイナリのみ |

## 主な機能

Python版と同等。

- **スムーズスクロール**: `REL_WHEEL_HI_RES`による高解像度スクロール
- **水平スクロール**: 2本指で縁をなぞると横スクロール
- **動的速度調整**: 角速度に応じてスクロール量を切り替え
- **慣性スクロール**: 指を離した後も減速しながらスクロールが持続
- **TOML設定**: デッドゾーン・感度・慣性をファイルで調整

## 他のLet's Noteモデルでも動く？

SV1で作ったが、`config.toml`の以下の値を調整すれば他のモデルでも動くはず。

| 設定キー | 内容 | 調べ方 |
| :--- | :--- | :--- |
| `device.name` | タッチパッドのデバイス名（部分一致） | `evtest` か `libinput list-devices` |
| `wheelpad.center_x` / `center_y` | ホイールパッドの中心座標 | `evtest`でパッド中央をタッチして確認 |
| `wheelpad.deadzone` | 中心の無反応ゾーン半径 | 中央付近で誤検知するなら大きくする |

## インストール

### 必要なもの

- Go 1.26以上（ソースからビルドする場合）
- systemd（`--user`サービスとして常駐させる場合）

### ビルド

```bash
go install github.com/kuroiko0429/wheelpad-go@latest
```

またはソースから:

```bash
git clone https://github.com/kuroiko0429/wheelpad-go.git
cd wheelpad-go
go build -o wheelpad-go
```

### セットアップスクリプト

`setup.sh`がビルド・systemd `--user`サービスの登録・udevルールの配置まで一括で行う（`git clone`したソースツリー内で実行する前提。`go install`だけの場合は使えない）。

```bash
./setup.sh
```

やっていること:

1. バイナリが無ければビルド
2. `~/.config/systemd/user/wheelpad.service`を生成
3. `/etc/udev/rules.d/99-wheelpad.rules`を配置（`sudo`が必要）してデバイスへのアクセス権を付与
4. `systemctl --user enable --now wheelpad.service`で起動

udevルールに`TAG+="uaccess"`を使っているため、Python版のように`usermod -aG input,uinput`でグループに追加する必要はない。ログイン中のユーザーに自動でアクセス権が付く。

:::message
`setup.sh`内の`ATTRS{name}=="Synaptics TM3562-003"`はデフォルトのデバイス名。自分のタッチパッド名が違う場合は`config.toml`の`device.name`と合わせて書き換える。
:::

### 設定ファイル

`/etc/wheelpad/config.toml`（`-c`で明示的に指定も可能）:

```toml
[device]
name = "Synaptics TM3562-003"

[wheelpad]
center_x = 264
center_y = 264
deadzone = 195
sensitivity = 0.3
hires_step = 60
natural_scroll = false

[speed]
thresholds = [
  { velocity = 8.0, multiplier = 4 },
  { velocity = 4.0, multiplier = 3 },
  { velocity = 2.0, multiplier = 2 },
]

[inertia]
enabled = true
friction = 0.85
min_velocity = 0.5
interval = 0.016
```

### 動作確認・デバッグ

```bash
systemctl --user status wheelpad.service
journalctl --user -u wheelpad.service -f
```

`--debug`フラグを付けて手動起動すると、タッチ検出・スクロール方向・速度がターミナルに出力される。

```bash
sudo ./wheelpad-go --config ./config.toml --debug
```

## ライセンス

[MIT License](LICENSE)
