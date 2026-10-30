/** 手柄键位提示的符号表。
 *
 * 后端（internal/pad）已经把按键语义统一成 Xbox 布局：
 * south = 下方那颗键，north = 上方，west = 左，east = 右。
 * 所以「启动 / 详情 / 收藏 / 返回」的位置固定，只有画的符号不同：
 *
 * - Xbox：A 启动、Y 详情、X 收藏、B 返回、LB / RB 换一组
 * - PlayStation：✕ 启动、△ 详情、□ 收藏、○ 返回、L1 / R1 换一组
 * - Switch：B 启动、X 详情、Y 收藏、A 返回、L / R 换一组（任天堂物理排布相反）
 */
export type PadKind = "xbox" | "playstation" | "switch";

export interface Glyph {
  /** 键帽上画的字：A / B / ✕ / △ …… */
  text: string;
  /** 配色用的类名：跟着真手柄的键色走 */
  tone: string;
}

export interface PadGlyphs {
  /** 左右肩键：主机上「换一组内容」的标准位置 */
  prev: string;
  next: string;
  confirm: Glyph;
  detail: Glyph;
  favorite: Glyph;
  back: Glyph;
  /** 中间那颗功能键：Menu / Options / + */
  menu: string;
  /** 功能键旁边那颗小键：View / Create / − （打开全部游戏） */
  view: string;
}

const TABLES: Record<PadKind, PadGlyphs> = {
  xbox: {
    prev: "LB",
    next: "RB",
    confirm: { text: "A", tone: "xb-green" },
    detail: { text: "Y", tone: "xb-yellow" },
    favorite: { text: "X", tone: "xb-blue" },
    back: { text: "B", tone: "xb-red" },
    menu: "Menu",
    view: "View",
  },
  playstation: {
    prev: "L1",
    next: "R1",
    confirm: { text: "✕", tone: "ps-blue" },
    detail: { text: "△", tone: "ps-green" },
    favorite: { text: "□", tone: "ps-pink" },
    back: { text: "○", tone: "ps-red" },
    menu: "Options",
    view: "Create",
  },
  switch: {
    prev: "L",
    next: "R",
    confirm: { text: "B", tone: "sw" },
    detail: { text: "X", tone: "sw" },
    favorite: { text: "Y", tone: "sw" },
    back: { text: "A", tone: "sw" },
    menu: "+",
    view: "−",
  },
};

/** 认不出来的类型一律按 Xbox 画（XInput 手柄最常见）。 */
export function glyphsFor(kind?: string): PadGlyphs {
  if (kind === "playstation" || kind === "switch") return TABLES[kind];
  return TABLES.xbox;
}
