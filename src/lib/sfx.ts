/**
 * 极简音效：全部用 WebAudio 现场合成，不打包任何音频文件。
 * 主机界面的「咔哒感」基本就靠移动 tick 和确认 whoosh。
 */
declare global {
  interface Window {
    /** 老 WebKit（macOS 上的 WKWebView）只有带前缀的构造器。 */
    webkitAudioContext?: typeof AudioContext;
  }
}

let ctx: AudioContext | null = null;
let enabled = true;

function audio(): AudioContext | null {
  if (!enabled) return null;
  if (!ctx) {
    const Ctor = window.AudioContext ?? window.webkitAudioContext;
    if (!Ctor) return null;
    ctx = new Ctor();
  }
  // 某些平台的 WebAudio 会自己挂起，用之前唤醒一下。
  if (ctx.state === "suspended") void ctx.resume();
  return ctx;
}

interface BlipOptions {
  freq: number;
  to?: number;
  duration: number;
  gain?: number;
  type?: OscillatorType;
}

function blip({ freq, to, duration, gain = 0.05, type = "sine" }: BlipOptions) {
  const ac = audio();
  if (!ac) return;

  const osc = ac.createOscillator();
  const amp = ac.createGain();
  const now = ac.currentTime;

  osc.type = type;
  osc.frequency.setValueAtTime(freq, now);
  if (to) osc.frequency.exponentialRampToValueAtTime(to, now + duration);

  // 快起慢落，听起来才像「咔哒」而不是「嘟——」。
  amp.gain.setValueAtTime(0.0001, now);
  amp.gain.exponentialRampToValueAtTime(gain, now + 0.008);
  amp.gain.exponentialRampToValueAtTime(0.0001, now + duration);

  osc.connect(amp).connect(ac.destination);
  osc.start(now);
  osc.stop(now + duration + 0.02);
}

export const sfx = {
  /** 光标移动。 */
  move: () => blip({ freq: 880, to: 1180, duration: 0.06, gain: 0.028, type: "triangle" }),
  /** 确认 / 启动。 */
  confirm: () => blip({ freq: 520, to: 990, duration: 0.18, gain: 0.05 }),
  /** 返回。 */
  back: () => blip({ freq: 460, to: 280, duration: 0.14, gain: 0.04, type: "triangle" }),
  /** 出错。 */
  error: () => blip({ freq: 220, to: 140, duration: 0.22, gain: 0.05, type: "sawtooth" }),
  setEnabled(on: boolean) {
    enabled = on;
  },
  get enabled() {
    return enabled;
  },
};
