import type { Game } from "../mygo";

/** 主色板：没有封面取色时的兜底，按标题散列挑一个，保证同一游戏永远同色。 */
const PALETTE = [
  "#c2762b",
  "#b23a48",
  "#2f6f8a",
  "#6a4a8a",
  "#2f8a6a",
  "#8a2f5a",
  "#7a6a2f",
  "#3a5a9a",
];

/** accentOf 返回一部游戏的主色：后端给了就用后端的，否则按标题散列。 */
export function accentOf(game?: Game): string {
  if (!game) return "#3a4a6a";
  if (game.accent) return game.accent;
  let hash = 0;
  for (const ch of game.title) hash = (hash * 31 + ch.codePointAt(0)!) % 997;
  return PALETTE[hash % PALETTE.length]!;
}

/** providerLabel 把 provider 变成界面上显示的名字。 */
export function providerLabel(provider: string): string {
  switch (provider) {
    case "steam":
      return "Steam";
    case "epic":
      return "Epic";
    case "wegame":
      return "WeGame";
    case "manual":
      return "手动添加";
    case "demo":
      return "演示数据";
    default:
      return provider;
  }
}

/** formatPlaytime 把分钟数说成人话。 */
export function formatPlaytime(minutes?: number): string {
  if (!minutes) return "";
  if (minutes < 60) return `${minutes} 分钟`;
  const hours = Math.round(minutes / 6) / 10;
  return `${hours} 小时`;
}

/** formatLastPlayed 返回「昨天 / 3 天前 / 2026-03-01」这样的相对时间。 */
export function formatLastPlayed(iso?: string): string {
  if (!iso) return "";
  const then = new Date(iso);
  if (Number.isNaN(then.getTime())) return "";
  const diffDays = Math.floor((Date.now() - then.getTime()) / 86_400_000);
  if (diffDays <= 0) return "今天";
  if (diffDays === 1) return "昨天";
  if (diffDays < 30) return `${diffDays} 天前`;
  return then.toLocaleDateString("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit" });
}

/** formatDate 返回「2026-10-29 14:03」这样的绝对时间，详情里用。 */
export function formatDate(iso?: string): string {
  if (!iso) return "";
  const then = new Date(iso);
  if (Number.isNaN(then.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${then.getFullYear()}-${pad(then.getMonth() + 1)}-${pad(then.getDate())} ${pad(then.getHours())}:${pad(then.getMinutes())}`;
}

/** formatSize 把字节说成 GB。 */
export function formatSize(bytes?: number): string {
  if (!bytes) return "";
  return `${Math.round((bytes / 1024 ** 3) * 10) / 10} GB`;
}

/**
 * vivid 把主色调成一个「鲜艳版」：固定饱和度与亮度、只保留色相。
 *
 * 原因：封面取色 / demo 配色常常是低饱和的土色（#8a6a2f 之类），
 * 直接铺背景会糊成一片泥。用 HSL 提饱和后才有 PS5 那种灯箱感。
 */
export function vivid(hex: string, saturation = 0.72, lightness = 0.46): string {
  const rgb = parseHex(hex);
  if (!rgb) return hex;
  const [h, , l] = rgbToHsl(rgb);
  // 原本就很暗的色（深灰蓝之类）稍微抬一点亮度，避免死黑
  const light = Math.min(0.62, Math.max(lightness, l * 0.9));
  return hslToHex(h, saturation, light);
}

type RGB = [number, number, number];

function parseHex(hex: string): RGB | null {
  const m = /^#?([\da-f]{3}|[\da-f]{6})$/i.exec(hex.trim());
  if (!m) return null;
  const body = m[1]!.length === 3 ? [...m[1]!].map((c) => c + c).join("") : m[1]!;
  const n = Number.parseInt(body, 16);
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

function rgbToHsl([r, g, b]: RGB): [number, number, number] {
  const rn = r / 255;
  const gn = g / 255;
  const bn = b / 255;
  const max = Math.max(rn, gn, bn);
  const min = Math.min(rn, gn, bn);
  const l = (max + min) / 2;
  const d = max - min;
  if (d === 0) return [0, 0, l];
  const s = l > 0.5 ? d / (2 - max - min) : d / (max + min);
  let h: number;
  if (max === rn) h = ((gn - bn) / d + (gn < bn ? 6 : 0)) / 6;
  else if (max === gn) h = ((bn - rn) / d + 2) / 6;
  else h = ((rn - gn) / d + 4) / 6;
  return [h, s, l];
}

function hslToHex(h: number, s: number, l: number): string {
  const hue = (h + 1) % 1;
  const q = l < 0.5 ? l * (1 + s) : l + s - l * s;
  const p = 2 * l - q;
  const to = (t: number) => {
    let v = t;
    if (v < 0) v += 1;
    if (v > 1) v -= 1;
    if (v < 1 / 6) return p + (q - p) * 6 * v;
    if (v < 1 / 2) return q;
    if (v < 2 / 3) return p + (q - p) * (2 / 3 - v) * 6;
    return p;
  };
  const channel = (t: number) =>
    Math.round(Math.min(1, Math.max(0, to(t))) * 255)
      .toString(16)
      .padStart(2, "0");
  return `#${channel(hue + 1 / 3)}${channel(hue)}${channel(hue - 1 / 3)}`;
}
