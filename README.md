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

`sudo ./setup.sh` ではなく通常のユーザーで実行する。システム側の変更が必要な箇所だけ、スクリプト内部で `sudo` を呼び出す。

やっていること:

1. バイナリが無ければビルド
2. `~/.config/systemd/user/wheelpad.service`を生成
3. `/etc/udev/rules.d/71-wheelpad.rules`を配置（`sudo`が必要）し、旧 `99-wheelpad.rules` を削除して udev ルールを再適用
4. systemd ユーザーサービスをリロードし、自動起動の有効化・失敗状態のリセット・再起動を実行

udevルールに`TAG+="uaccess"`を使っているため、Python版のように`usermod -aG input,uinput`でグループに追加する必要はない。ローカルのアクティブセッションのユーザーにアクセス権が付く。

`uaccess` の ACL 付与は systemd の `73-seat-late.rules` で実行されるため、タグを付けるルールはそれより辞書順で前に置く必要がある。旧 `99-wheelpad.rules` ではタグの付与が遅く、グループ権限などでアクセスできない環境では起動に失敗する。

セッションのアクティブ化による ACL 付与が起動に間に合わない場合に備え、生成するサービスは `RestartSec=5`、`StartLimitIntervalSec=0` としている。恒久的な設定ミスでも再試行は続くため、起動できない場合はログを確認する。

#### 以前のバージョンからの更新

更新したソースツリーで以下を実行する。旧ルールの削除とサービス設定の更新も `setup.sh` が行うため、手動でのリネームは不要。

```bash
go build -o wheelpad-go
./setup.sh
```

設定ファイルは上書きしない。サービスは再起動されるため、操作中のタッチパッド入力が一時的に途切れることがある。

:::message
`setup.sh`内の`ATTRS{name}=="Synaptics TM3562-003"`はデフォルトのデバイス名。自分のタッチパッド名が違う場合は`config.toml`の`device.name`と合わせて書き換える。
:::

### タッチパッドの個別設定について

デーモンは物理タッチパッドを排他取得し、通常の入力を `LetsNote-Virtual-Pad`、スクロールを `LetsNote-Virtual-Wheel` から送信する。
そのため、KDE のタッチパッド設定や Hyprland のデバイス別設定など、物理デバイスに紐づく設定はそのまま引き継がれないことがある。導入後は仮想デバイスに対してタップ・加速度・スクロール方向などを設定し直す。
コンポジタが使うデバイス識別名は表示名と異なる場合があるため、各環境のデバイス一覧で確認する。

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
# Optional: a lower stopping speed lets deliberate flicks fade out gently.
# When omitted or <= 0, uses min_velocity (original behavior).
# stop_velocity = 0.2
interval = 0.016
```

### 動作確認・デバッグ

```bash
systemctl --user status wheelpad.service
journalctl --user -u wheelpad.service -f
```

`--debug`フラグを付けて手動起動すると、タッチ検出・スクロール方向・速度に加え、開けなかった入力デバイスのパスとエラーがターミナルに出力される。
先にサービスを停止すること。起動中のサービスと同時に実行すると、デバイスの排他取得が `device or resource busy` で失敗する。

```bash
systemctl --user stop wheelpad.service
./wheelpad-go --debug
```

設定ファイルを明示する場合は `--config /path/to/config.toml` を追加する。通常は `sudo` 不要で、root で起動すると通常ユーザーのアクセス権の問題を見逃すことがある。
`開けません（スキップ）` は対象外の入力デバイスでも表示されるため、対象タッチパッドのイベントノードと照合する。

確認後は `Ctrl+C` で手動起動を終了し、サービスを再開する。

```bash
systemctl --user start wheelpad.service
```

## ライセンス

[MIT License](LICENSE)
