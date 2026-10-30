#!/usr/bin/env python3
"""shot.py —— 可靠地截取运行中应用的画面。

为什么需要它：WebView（macOS 的 WKWebView）在窗口不在前台/被遮挡时**不提交新帧**，
`capture` 拿到的还是上一次提交的旧画面。之前几次截图看到「旧帧」，就是
窗口在后台而脚本照样抓图导致的。

这个脚本做三件事：
1. 用 System Events 把目标进程抢回前台；
2. 轮询页面里的 `document.hasFocus()`，确认真的在前台（拿不到就报错退出，
   绝不静默保存旧帧）；
3. 强制一次重绘（改 opacity 触发合成层更新）后再 capture。

用法：
    python3 scripts/shot.py --sock /tmp/gl.sock --out build/shots/x.png
    python3 scripts/shot.py --out x.png --before "window.dispatchEvent(new KeyboardEvent('keydown',{key:'g'}))"
"""

import argparse
import base64
import json
import os
import socket
import subprocess
import sys
import time

FOCUS_TIMEOUT = 4.0


class App:
    def __init__(self, sock_path: str):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.connect(sock_path)
        self.file = self.sock.makefile("rwb")
        self.seq = 0

    def call(self, method: str, **params):
        self.seq += 1
        payload = json.dumps({"id": self.seq, "method": method, "params": params})
        self.file.write((payload + "\n").encode())
        self.file.flush()
        raw = self.file.readline()
        if not raw:
            raise RuntimeError(f"应用没有回应 {method}（socket 断了？）")
        try:
            reply = json.loads(raw)
        except json.JSONDecodeError as err:
            raise RuntimeError(f"{method} 的回应不是 JSON: {raw[:200]!r}") from err
        if "error" in reply:
            raise RuntimeError(f"{method} 失败: {reply['error']}")
        return reply.get("result")

    def eval(self, script: str):
        return self.call("eval", script=script).get("value")


def frontmost(process_name: str) -> None:
    """把进程抢到前台（失败不致命，后面还会轮询 hasFocus）。"""
    script = f'tell application "System Events" to set frontmost of first process whose name is "{process_name}" to true'
    subprocess.run(["osascript", "-e", script], capture_output=True, check=False)


def wait_for_focus(app: App, process_name: str, timeout: float = FOCUS_TIMEOUT) -> bool:
    deadline = time.time() + timeout
    while time.time() < deadline:
        frontmost(process_name)
        time.sleep(0.25)
        try:
            if app.eval("document.hasFocus()"):
                return True
        except RuntimeError:
            pass
    return False


def main() -> int:
    ap = argparse.ArgumentParser(description="截取运行中应用的画面（会先确保窗口在前台）")
    # 默认从 MYGO_AUTOMATION 取，不写死某个临时目录
    ap.add_argument("--sock", default=os.environ.get("MYGO_AUTOMATION", ""), help="automation socket 路径")
    ap.add_argument("--out", required=True, help="PNG 输出路径")
    ap.add_argument("--process", default="Game Launcher", help="进程名（System Events 用）")
    ap.add_argument("--before", default="", help="截图前要执行的 JS（可选）")
    ap.add_argument("--settle", type=float, default=0.6, help="重绘后等待秒数")
    args = ap.parse_args()

    if not args.sock:
        print("✗ 缺少 socket 路径：用 --sock 指定，或先 export MYGO_AUTOMATION=<路径>", file=sys.stderr)
        return 2

    try:
        app = App(args.sock)
    except OSError as err:
        print(f"✗ 连不上 {args.sock}: {err}", file=sys.stderr)
        return 2

    if args.before:
        app.eval(f"{args.before};true")
        time.sleep(0.4)

    if not wait_for_focus(app, args.process):
        print("✗ 拿不到窗口焦点：WebView 在后台不会提交新帧，抓到的一定是旧画面。", file=sys.stderr)
        print("  请确认应用在跑、并且允许 System Events 控制本机。", file=sys.stderr)
        return 1

    # 强制一次重绘：改动合成属性会让 WebKit 重新提交一帧
    app.eval(
        "(() => { const el = document.body;"
        "el.style.opacity = '0.999';"
        "requestAnimationFrame(() => { el.style.opacity = ''; });"
        "return true; })()"
    )
    time.sleep(args.settle)

    captured = app.call("capture")
    if not captured or "png" not in captured:
        print("✗ capture 没有返回图片数据", file=sys.stderr)
        return 1

    png = base64.b64decode(captured["png"])
    try:
        with open(args.out, "wb") as handle:
            handle.write(png)
    except OSError as err:
        print(f"✗ 写文件失败 {args.out}: {err}", file=sys.stderr)
        return 1

    print(f"✓ 已保存 {args.out}（{len(png) // 1024} KB，窗口在前台）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
