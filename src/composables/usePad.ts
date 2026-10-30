import { onMounted, onUnmounted } from "vue";
import { Channel } from "mygo-runtime";
import { Launcher, type State } from "../mygo";

/** 手柄给界面的一次意图：要么移动焦点，要么按了个动作键。 */
export type PadIntent =
  | { kind: "nav"; dir: "up" | "down" | "left" | "right" }
  | { kind: "button"; button: string };

const REPEAT_DELAY = 380; // 按住多久开始连发
const REPEAT_INTERVAL = 130; // 连发间隔

/**
 * usePad 订阅 Go 侧推来的手柄帧，把「按了哪个键」变成界面意图。
 *
 * 后端只发离散帧（哪几个键按住、摇杆推到哪个方向），这里负责：
 * - 按键的边沿检测（按住不会重复触发动作键）
 * - 方向键/摇杆的连发（按住一直翻页）
 */
export function usePad(onIntent: (intent: PadIntent) => void, onKind?: (kind: string) => void) {
  let stopped = false;
  let channel: Channel<State[]> | null = null;
  const prevButtons = new Map<string, Set<string>>();
  let lastKind = "";

  let currentDir = "";
  let delayTimer: number | undefined;
  let repeatTimer: number | undefined;

  function clearTimers() {
    if (delayTimer !== undefined) window.clearTimeout(delayTimer);
    if (repeatTimer !== undefined) window.clearInterval(repeatTimer);
    delayTimer = undefined;
    repeatTimer = undefined;
  }

  function setDir(next: string) {
    if (next === currentDir) return;
    currentDir = next;
    clearTimers();
    if (!next) return;

    const dir = next as "up" | "down" | "left" | "right";
    onIntent({ kind: "nav", dir });
    delayTimer = window.setTimeout(() => {
      repeatTimer = window.setInterval(() => onIntent({ kind: "nav", dir }), REPEAT_INTERVAL);
    }, REPEAT_DELAY);
  }

  function applyFrame(states: State[]) {
    const connected = new Set<string>();
    let dir = "";

    for (const pad of states) {
      if (!pad.connected) continue;
      connected.add(pad.name);

      // 手柄类型（xbox / playstation / switch）变了就通知界面换键位图
      if (pad.kind && pad.kind !== lastKind) {
        lastKind = pad.kind;
        onKind?.(pad.kind);
      }

      const before = prevButtons.get(pad.name) ?? new Set<string>();
      for (const button of pad.buttons) {
        if (!before.has(button)) onIntent({ kind: "button", button });
      }
      prevButtons.set(pad.name, new Set(pad.buttons));

      if (pad.dir) dir = pad.dir; // 多只手柄时以最后一只的方向为准，够用
    }

    for (const name of [...prevButtons.keys()]) {
      if (!connected.has(name)) prevButtons.delete(name);
    }
    setDir(dir);
  }

  async function loop() {
    while (!stopped) {
      channel = new Channel<State[]>();
      try {
        const done = Launcher.pads(channel);
        for await (const states of channel) {
          if (stopped) break;
          applyFrame(states);
        }
        await done;
      } catch {
        // 窗口还没准备好、或者 Go 侧重启：等一下再连，不打断界面。
      }
      if (stopped) break;
      await new Promise((resolve) => setTimeout(resolve, 800));
    }
  }

  // ---- 端到端测试用的帧注入 ----
  // 测试脚本通过 eval 往 window.__padQueue 里推帧，这里取出来喂给 applyFrame，
  // 走的是**和真实手柄完全一样**的管线（边沿检测、方向连发都能测到）。
  // 生产环境里这个队列永远是 undefined，所以这段代码不会真的做事。
  type PadWindow = Window & { __padQueue?: State[][] };
  let drainTimer: number | undefined;

  function drainDebugQueue() {
    const queue = (window as PadWindow).__padQueue;
    if (!queue?.length) return;
    while (queue.length) applyFrame(queue.shift() as State[]);
  }

  onMounted(() => {
    void loop();
    drainTimer = window.setInterval(drainDebugQueue, 30);
  });

  onUnmounted(() => {
    stopped = true;
    clearTimers();
    if (drainTimer !== undefined) window.clearInterval(drainTimer);
    channel?.close();
  });
}
