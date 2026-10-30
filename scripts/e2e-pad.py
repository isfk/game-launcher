#!/usr/bin/env python3
"""e2e-pad.py —— 用注入的手柄帧驱动界面，验证「手柄意图管线」。

为什么需要它：真实手柄没法放进 CI，也没法在自动化里按。这里把帧直接推进
`window.__padQueue`，`usePad` 会像收到真实手柄帧一样处理（边沿检测、方向连发
都会跑到），所以测的是**真实管线**，不是另一条测试专用通道。

用法：
    # 1) 起一个可被驱动的应用（e2e 标签，发布产物不含）
    GOFLAGS=-tags=e2e mygo build -debug -skip-dmg -skip-notarize
    LAUNCHER_STEAM_DIR=/tmp/fakesteam MYGO_AUTOMATION=/tmp/gl.sock \
      "build/darwin-arm64/Game Launcher.app/Contents/MacOS/Game Launcher" &

    # 2) 跑这个脚本
    python3 scripts/e2e-pad.py /tmp/gl.sock
"""

import json
import os
import socket
import sys
import time

# socket 路径从参数或 MYGO_AUTOMATION 来：不写死目录，
# 免得脚本在共享机器上连到别人留下的 socket。
SOCK = sys.argv[1] if len(sys.argv) > 1 else os.environ.get("MYGO_AUTOMATION", "")

FAILED = []


def connect():
    s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    s.connect(SOCK)
    return s


class App:
    def __init__(self, sock):
        self.sock = sock
        self.file = sock.makefile("rwb")
        self.seq = 0

    def call(self, method, **params):
        self.seq += 1
        self.file.write((json.dumps({"id": self.seq, "method": method, "params": params}) + "\n").encode())
        self.file.flush()

        raw = self.file.readline()
        if not raw:
            raise RuntimeError(f"应用没有回应 {method}（socket 断了？）")
        try:
            reply = json.loads(raw)
        except json.JSONDecodeError as err:
            raise RuntimeError(f"{method} 的回应不是合法 JSON: {raw[:200]!r}") from err
        if "error" in reply:
            raise RuntimeError(f"{method} 失败: {reply['error']}")
        return reply.get("result")

    def eval(self, script):
        return self.call("eval", script=script).get("value")

    # ---- 手柄帧注入：和真实帧同一条管线 ----
    def pad(self, buttons=(), direction="", name="Xbox 手柄", kind="xbox", connected=True):
        frame = {
            "name": name,
            "kind": kind,
            "buttons": list(buttons),
            "dir": direction,
            "connected": connected,
        }
        self.eval(
            "window.__padQueue = window.__padQueue || [];"
            f"window.__padQueue.push({json.dumps([frame])});true"
        )
        time.sleep(0.12)  # 让 usePad 的 drain 定时器（30ms）取走

    def release(self, name="Xbox 手柄"):
        """松手：推一帧空按键，边沿检测才会允许下一次按下再触发。"""
        self.pad(buttons=(), direction="", name=name)

    def press(self, button, hold=0.12):
        self.pad(buttons=[button])
        time.sleep(hold)
        self.release()
        time.sleep(0.35)


def focused(app):
    return app.eval("document.querySelector('.card.focused .name')?.textContent?.trim() ?? null")


def check(name, got, want):
    ok = got == want
    print(f"  {'✓' if ok else '✗'} {name}: {got!r}" + ("" if ok else f"（期望 {want!r}）"))
    if not ok:
        FAILED.append(name)


def main():
    if not SOCK:
        print("用法: python3 scripts/e2e-pad.py <automation socket 路径>", file=sys.stderr)
        print("   或先 export MYGO_AUTOMATION=<路径>", file=sys.stderr)
        return 2

    app = App(connect())

    # 清掉可能残留的浮层，从干净的首页开始
    for _ in range(4):
        app.call("eval", script="window.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape'}));true")
        time.sleep(0.15)

    before = focused(app)
    print("虚拟手柄 E2E")
    print(f"  起始卡片: {before!r}")

    print("\n[1] 方向键 → 焦点右移")
    app.pad(direction="right")
    time.sleep(0.2)
    app.release()
    time.sleep(0.4)
    after = focused(app)
    if after == before:
        check("焦点变化", after, "换一张卡")
    else:
        print(f"  ✓ 焦点移动: {before!r} → {after!r}")

    print("\n[2] 按住方向 → 连发翻页（REPEAT_DELAY 380ms 后每 130ms 一次）")
    app.pad(direction="right")
    time.sleep(0.8)  # 超过 380ms 延迟，应该已经连发几次
    app.release()
    time.sleep(0.4)
    repeated = focused(app)
    print(f"  {'✓' if repeated != after else '✗'} 连发后焦点: {repeated!r}")
    if repeated == after:
        FAILED.append("方向连发")

    print("\n[3] Y / north → 打开详情")
    app.press("north")
    time.sleep(0.5)
    check("详情打开", app.eval("!!document.querySelector('.panel.open')"), True)

    print("\n[4] B / east → 收起详情")
    app.press("east")
    time.sleep(0.5)
    check("详情关闭", app.eval("!!document.querySelector('.panel.open')"), False)

    print("\n[5] X / west → 收藏切换")
    fav_before = app.eval("!!document.querySelector('.card.focused')?.classList.contains('favorite')")
    app.press("west")
    time.sleep(0.6)
    fav_after = app.eval("!!document.querySelector('.card.focused')?.classList.contains('favorite')")
    print(f"  {'✓' if fav_after != fav_before else '✗'} 收藏: {fav_before} → {fav_after}")
    if fav_after == fav_before:
        FAILED.append("收藏切换")
    # 还原，免得污染后面的运行
    app.press("west")
    time.sleep(0.4)

    print("\n[6] Options / start → 打开设置")
    app.press("start")
    time.sleep(0.6)
    check("设置打开", app.eval("!!document.querySelector('.panel.open')"), True)

    print("\n[7] 设置里 ↓ 移动焦点")
    row_before = app.eval("document.querySelector('.panel.open .btn.focused')?.textContent?.trim() ?? null")
    app.pad(direction="down")
    time.sleep(0.2)
    app.release()
    time.sleep(0.4)
    row_after = app.eval("document.querySelector('.panel.open .btn.focused')?.textContent?.trim() ?? null")
    print(f"  {'✓' if row_after != row_before else '✗'} 焦点行: {row_before!r} → {row_after!r}")
    if row_after == row_before:
        FAILED.append("设置内竖向导航")

    print("\n[8] B / east → 关闭设置")
    app.press("east")
    time.sleep(0.5)
    check("设置关闭", app.eval("!!document.querySelector('.panel.open')"), False)

    print("\n[9] 平台徽章键位跟随手柄类型")
    app.pad(buttons=(), kind="playstation", name="DualSense")
    time.sleep(0.4)
    kinds = app.eval("document.body.className")
    print(f"  手柄类型切换后 body class: {kinds!r}")
    app.release(name="DualSense")

    print()
    if FAILED:
        print(f"✗ {len(FAILED)} 项失败: {', '.join(FAILED)}")
        return 1
    print("✓ 全部通过")
    return 0


if __name__ == "__main__":
    sys.exit(main())
