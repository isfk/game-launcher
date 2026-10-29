"""端到端验证：在 Steam 里装完一部游戏后，启动器不用重启就该自动出现在首页。

用法（先在项目根目录）：

    GOFLAGS=-tags=e2e mygo build -debug -skip-dmg -skip-notarize
    rm -f "$SOCK"
    LAUNCHER_STEAM_DIR=/path/to/fakesteam MYGO_AUTOMATION=$SOCK \\
        "build/darwin-arm64/Game Launcher.app/Contents/MacOS/Game Launcher" &
    python3 scripts/e2e-library-watch.py --steam-dir /path/to/fakesteam --sock $SOCK

退出码 0 表示新游戏自动出现了。
"""

from __future__ import annotations

import argparse
import contextlib
import json
import os
import socket
import sys
import time

TITLES_SCRIPT = "[...document.querySelectorAll('.card .name')].map(e => e.textContent)"


class Client:
    """automation 插件的最小客户端（见 docs/plugins/automation.md）。"""

    def __init__(self, sock_path: str) -> None:
        self.sock = None
        for _ in range(60):
            try:
                conn = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
                conn.connect(sock_path)
            except OSError:
                time.sleep(0.3)
                continue
            self.sock = conn
            break
        if self.sock is None:
            raise SystemExit(f"连不上自动化 socket: {sock_path}")
        self.stream = self.sock.makefile("rwb")
        self.calls = 0

    def call(self, method: str, **params: object) -> object:
        self.calls += 1
        request = {"id": self.calls, "method": method, "params": params}
        self.stream.write((json.dumps(request) + "\n").encode())
        self.stream.flush()
        line = self.stream.readline()
        if not line:
            raise SystemExit("socket 断了")
        try:
            response = json.loads(line)
        except json.JSONDecodeError as err:
            raise SystemExit(f"返回的不是 JSON: {line[:80]!r}") from err
        if "error" in response:
            raise SystemExit(f"{method} 出错: {response['error']}")
        return response["result"]

    def titles(self) -> list[str]:
        result = self.call("eval", script=TITLES_SCRIPT)
        if not isinstance(result, dict):
            return []
        value = result.get("value")
        return value if isinstance(value, list) else []

    def close(self) -> None:
        try:
            self.stream.close()
            if self.sock is not None:
                self.sock.close()
        except OSError:
            pass


def write_manifest(steam_dir: str, appid: str, name: str) -> str:
    """往假库里塞一份 appmanifest，模拟「Steam 刚装完一部游戏」。"""
    path = os.path.join(steam_dir, "steamapps", f"appmanifest_{appid}.acf")
    body = (
        '"AppState"\n{\n'
        f'\t"appid"\t\t"{appid}"\n'
        f'\t"name"\t\t"{name}"\n'
        '\t"StateFlags"\t\t"4"\n'
        f'\t"installdir"\t\t"{name}"\n'
        '\t"SizeOnDisk"\t\t"51539607552"\n'
        '\t"LastPlayed"\t\t"1793000000"\n}\n'
    )
    try:
        with open(path, "w", encoding="utf-8") as handle:
            handle.write(body)
    except OSError as err:
        raise SystemExit(f"写清单失败: {err}") from err
    return path


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--steam-dir", required=True, help="假 Steam 库根目录")
    parser.add_argument("--sock", required=True, help="MYGO_AUTOMATION 的 socket 路径")
    parser.add_argument("--appid", default="2358720", help="要假装装上的 appid")
    parser.add_argument("--name", default="Black Myth: Wukong", help="它的名字")
    parser.add_argument("--timeout", type=float, default=25.0, help="等多久算失败")
    args = parser.parse_args()

    client = Client(args.sock)
    path = ""
    try:
        before = client.titles()
        print("装之前:", before)

        path = write_manifest(args.steam_dir, args.appid, args.name)
        print(f"已写入 {os.path.basename(path)}（模拟在 Steam 里装完）")

        deadline = time.monotonic() + args.timeout
        after = before
        while time.monotonic() < deadline:
            time.sleep(1)
            after = client.titles()
            if any(args.name.split(":")[0] in title for title in after):
                break
        print("装之后:", after)

        found = any(args.name.split(":")[0] in title for title in after)
        print("RESULT:", "PASS 新游戏自动出现了" if found else "FAIL 没有自动出现")
        return 0 if found else 1
    finally:
        if path:
            with contextlib.suppress(OSError):
                os.remove(path)
        client.close()


if __name__ == "__main__":
    sys.exit(main())
