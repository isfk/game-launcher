<script setup lang="ts">
import { computed, ref } from "vue";
import { usePanelNav } from "../composables/usePanelNav";
import {
  accentOf,
  formatDate,
  formatLastPlayed,
  formatPlaytime,
  formatSize,
  providerLabel,
} from "../lib/format";
import { glyphsFor } from "../lib/glyphs";
import type { Game } from "../mygo";
import PanelShell from "./PanelShell.vue";

/**
 * DetailDrawer 是首页右侧滑出的游戏详情面板。
 *
 * 排版思路（照主机系统那套来，不照网页后台那套）：
 *   1. 图打头：封面铺满上半截，底下渐渐化进面板底色，LOGO 压在图上；
 *   2. 数字靠字排，不靠卡片：三个关键数字用发丝线分开，标签小、数值大；
 *   3. 次要信息收进「详情」段，发丝线行 + 等宽字体，长路径中间省略；
 *   4. 操作区固定在底部，启动是唯一的主按钮。
 *
 * 用抽屉而不是弹窗：弹窗会遮住卡片带，把「我选了哪张卡」这个上下文切断。
 */
const props = defineProps<{
  game?: Game;
  favorite?: boolean;
  /** 展开（滑入）/ 收起（滑出屏幕外） */
  open?: boolean;
  pad?: boolean;
  kind?: "xbox" | "playstation" | "switch";
  /** 卸载的二次确认：true 时底部换成确认条 */
  confirming?: boolean;
  /** 确认条展示中（请求在飞），按钮禁用 */
  busy?: boolean;
}>();

const emit = defineEmits<{
  close: [];
  launch: [];
  favorite: [];
  /** 请求进入卸载确认 */
  uninstall: [];
  /** 确认卸载 */
  "confirm-uninstall": [];
  /** 取消卸载 */
  "cancel-uninstall": [];
}>();

const logoFailed = ref(false);
const coverFailed = ref(false);

const logo = computed(() => (logoFailed.value ? "" : (props.game?.artwork?.logo ?? "")));

/** 挂掉的地址记下来，下次换游戏才重试 —— 别跟一张图死磕。 */
const art = computed(() => props.game?.artwork);
const cover = computed(() => (coverFailed.value ? "" : (art.value?.cover || art.value?.hero || "")));

const keys = computed(() => {
  const g = glyphsFor(props.kind);
  if (props.pad) return { launch: g.confirm.text, favorite: g.favorite.text, close: g.back.text };
  return { launch: "Enter", favorite: "X", close: "Esc" };
});

const eyebrow = computed(() => {
  const g = props.game;
  if (!g) return "";
  const parts = [providerLabel(g.provider)];
  if (g.running) parts.push("运行中");
  if (props.favorite) parts.push("已收藏");
  return parts.join(" · ");
});

/** 三个关键数字：缺的那个自己消失，不留一排「—」。 */
const trio = computed(() => {
  const g = props.game;
  if (!g) return [];
  return [
    { label: "总时长", value: formatPlaytime(g.playtimeMinutes) },
    { label: "上次游玩", value: formatLastPlayed(g.lastPlayed) },
    { label: "占用空间", value: formatSize(g.sizeBytes) },
  ].filter((s) => s.value);
});

const tags = computed(() => props.game?.tags ?? []);

const specs = computed(() => {
  const g = props.game;
  if (!g) return [];
  return [
    { label: "AppID", value: g.appId || "—", mono: true },
    { label: "上次启动", value: formatDate(g.lastPlayed) || "—", mono: true },
    { label: "安装位置", value: g.installDir || "—", mono: true, path: true },
  ];
});

/** 只有 Steam 的游戏能交给客户端卸载；演示数据没有可卸载的东西。 */
const canUninstall = computed(() => props.game?.provider === "steam" && !!props.game?.appId);
/** 手动添加的游戏只能「移除」：删的是一条记录，磁盘上的文件不动。 */
const canRemove = computed(() => props.game?.provider === "manual");
/** 底部要不要给「卸载 / 移除」留位置。 */
const canDestruct = computed(() => canUninstall.value || canRemove.value);

const destructLabel = computed(() => (canRemove.value ? "移除" : "卸载"));

/**
 * 面板内的动作顺序：启动在最前（默认焦点），卸载垫在最后。
 * 用 computed 包一层，游戏换了（比如没了卸载按钮）焦点会自动收敛。
 */
const nav = usePanelNav(() => {
  const items = [
    { key: "launch", activate: () => emit("launch") },
    { key: "favorite", activate: () => emit("favorite") },
  ];
  if (canDestruct.value) items.push({ key: "uninstall", activate: () => emit("uninstall") });
  items.push({ key: "close", activate: () => emit("close") });
  return items;
});

defineExpose({ move: nav.move, activate: nav.activate, reset: nav.reset, isFocused: nav.isFocused });

/** 长路径中间省略：尾部（游戏目录名）比开头的盘符有用得多。 */
function shortenPath(path: string, head = 14, tail = 22): string {
  if (path.length <= head + tail + 1) return path;
  return `${path.slice(0, head)}…${path.slice(-tail)}`;
}
</script>

<template>
  <PanelShell :open="open" :accent="accentOf(game)" label="游戏详情">
    <div class="scroll">
      <div class="cover">
        <img v-if="cover" :src="cover" alt="" @error="coverFailed = true" />
        <div v-else class="cover-blank">{{ (game?.title ?? "?").slice(0, 1) }}</div>
        <div class="veil" />
        <div class="cover-copy">
          <img v-if="logo" class="logo" :src="logo" alt="" @error="logoFailed = true" />
          <h2 v-else class="title">{{ game?.title ?? "—" }}</h2>
        </div>
      </div>

      <div class="content">
        <p class="eyebrow">{{ eyebrow }}</p>

        <dl v-if="trio.length" class="trio">
          <div v-for="item in trio" :key="item.label">
            <dt>{{ item.label }}</dt>
            <dd>{{ item.value }}</dd>
          </div>
        </dl>

        <ul v-if="tags.length" class="tags">
          <li v-for="tag in tags" :key="tag">{{ tag }}</li>
        </ul>

        <section v-if="specs.length" class="specs">
          <h3>详情</h3>
          <dl>
            <div v-for="spec in specs" :key="spec.label" class="spec">
              <dt>{{ spec.label }}</dt>
              <dd :class="{ mono: spec.mono, path: spec.path }" :title="spec.value">
                {{ spec.path ? shortenPath(spec.value) : spec.value }}
              </dd>
            </div>
          </dl>
        </section>
      </div>
    </div>

    <footer class="actions">
      <!-- 二次确认：卸载是不可逆的重操作，先在这里停一下再交给 Steam -->
      <template v-if="confirming">
        <p class="confirm-title">{{ destructLabel }}「{{ game?.title }}」？</p>
        <p v-if="canRemove" class="confirm-sub">
          只从启动器里移除这条记录，磁盘上的文件不动。想彻底删掉请自己删文件夹。
        </p>
        <p v-else class="confirm-sub">
          会交给 Steam 客户端执行，它还会再问一次。卸载完成后这张卡会自动消失。
        </p>
        <div class="row">
          <button class="act danger" :disabled="busy" @click="emit('confirm-uninstall')">
            {{ busy ? "处理中…" : canRemove ? "确认移除" : "确认卸载" }}
          </button>
          <button class="act ghost" :disabled="busy" @click="emit('cancel-uninstall')">取消</button>
        </div>
      </template>

      <template v-else>
        <button
          class="act primary"
          :class="{ focused: nav.isFocused('launch') }"
          @mouseenter="nav.focusTo(0)"
          @click="emit('launch')"
        >
          {{ game?.running ? "已在运行" : "启动游戏" }}
          <i class="glyph">{{ keys.launch }}</i>
        </button>
        <div class="row">
          <button
            class="act ghost"
            :class="{ on: favorite, focused: nav.isFocused('favorite') }"
            @mouseenter="nav.focusTo(1)"
            @click="emit('favorite')"
          >
            <i class="glyph">{{ keys.favorite }}</i>
            {{ favorite ? "已收藏" : "收藏" }}
          </button>
          <button
            v-if="canDestruct"
            class="act ghost danger-text"
            :class="{ focused: nav.isFocused('uninstall') }"
            @mouseenter="nav.focusTo(2)"
            @click="emit('uninstall')"
          >
            {{ destructLabel }}
          </button>
          <button
            class="act ghost"
            :class="{ focused: nav.isFocused('close') }"
            @mouseenter="nav.focusTo(canDestruct ? 3 : 2)"
            @click="emit('close')"
          >
            <i class="glyph">{{ keys.close }}</i>
            收起
          </button>
        </div>
      </template>
    </footer>
  </PanelShell>
</template>

<style scoped>
/* ---------- 滚动区 ---------- */

.scroll {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
  /* 面板本来就窄，滚动条那一条白线太扎眼，直接藏掉 */
  scrollbar-width: none;
}

.scroll::-webkit-scrollbar {
  width: 0;
}

/* ---------- 封面 ---------- */

.cover {
  position: relative;
  height: min(56vh, calc(var(--drawer-w, min(28rem, 32vw)) * 1.35));
  overflow: hidden;
}

.cover img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
  /* 竖版封面裁成横幅时保留上半部分：字标和人物头都在那儿 */
  object-position: center top;
}

/* 左内缘压暗，别让封面在面板边上「一刀切」 */
.cover::after {
  content: "";
  position: absolute;
  inset: 0 auto 0 0;
  width: 4.5rem;
  pointer-events: none;
  background: linear-gradient(90deg, rgba(9, 11, 16, 0.92) 0%, rgba(9, 11, 16, 0) 100%);
}

.cover-blank {
  display: grid;
  place-items: center;
  height: 100%;
  font-size: 3.4rem;
  font-weight: 700;
  color: rgba(255, 255, 255, 0.18);
  background: rgba(255, 255, 255, 0.04);
}

/* 从图化进面板底色：只在最后一段收紧，图的细节尽量留住 */
.veil {
  position: absolute;
  inset: 0;
  background: linear-gradient(
    180deg,
    rgba(9, 11, 16, 0.05) 0%,
    rgba(9, 11, 16, 0.12) 46%,
    rgba(9, 11, 16, 0.62) 78%,
    rgba(9, 11, 16, 0.96) 94%,
    rgb(9, 11, 16) 100%
  );
}

.cover-copy {
  position: absolute;
  left: 1.4rem;
  right: 1.4rem;
  bottom: 0.4rem;
}

.logo {
  display: block;
  max-width: 100%;
  max-height: 3rem;
  object-fit: contain;
  object-position: left bottom;
  filter: drop-shadow(0 8px 24px rgba(0, 0, 0, 0.7));
}

.title {
  margin: 0;
  font-size: 1.4rem;
  line-height: 1.1;
  letter-spacing: -0.02em;
  text-shadow: 0 6px 24px rgba(0, 0, 0, 0.7);
}

/* ---------- 正文 ---------- */

.content {
  padding: 0.7rem 1.4rem 1.2rem;
}

.eyebrow {
  margin: 0 0 1rem;
  font-size: 0.7rem;
  letter-spacing: 0.16em;
  text-transform: uppercase;
  color: color-mix(in oklab, var(--accent) 55%, var(--fg));
  opacity: 0.85;
}

/* 三个数字：发丝线分隔，标签小、数值大 —— 不用卡片 */
.trio {
  display: flex;
  margin: 0 0 1.1rem;
  padding: 0;
}

.trio > div {
  flex: 1 1 0;
  min-width: 0;
  padding-left: 0.9rem;
  border-left: 1px solid rgba(255, 255, 255, 0.12);
}

.trio > div:first-child {
  padding-left: 0;
  border-left: 0;
}

.trio dt {
  font-size: 0.64rem;
  letter-spacing: 0.08em;
  color: var(--faint);
}

.trio dd {
  margin: 0.25rem 0 0;
  font-size: 1.02rem;
  font-weight: 600;
  letter-spacing: -0.01em;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.tags {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
  margin: 0 0 1.1rem;
  padding: 0;
  list-style: none;
}

.tags li {
  padding: 0.22em 0.7em;
  border-radius: 999px;
  font-size: 0.7rem;
  color: var(--muted);
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(255, 255, 255, 0.09);
}

/* ---------- 详情段 ---------- */

.specs h3 {
  margin: 0 0 0.15rem;
  font-size: 0.64rem;
  font-weight: 500;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--faint);
}

.specs dl {
  margin: 0;
}

.spec {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 1rem;
  padding: 0.5rem 0;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.spec:last-child {
  border-bottom: 0;
}

.spec dt {
  flex: 0 0 auto;
  font-size: 0.74rem;
  color: var(--faint);
}

.spec dd {
  margin: 0;
  min-width: 0;
  font-size: 0.76rem;
  text-align: right;
  color: var(--muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.spec dd.mono {
  font-family: ui-monospace, "SF Mono", Menlo, monospace;
  font-size: 0.72rem;
  font-variant-numeric: tabular-nums;
}

/* ---------- 操作区 ---------- */

.actions {
  flex: 0 0 auto;
  display: flex;
  flex-direction: column;
  gap: 0.45rem;
  padding: 0.9rem 1.4rem calc(6vh + 0.5rem);
  border-top: 1px solid rgba(255, 255, 255, 0.07);
  background: linear-gradient(180deg, rgba(8, 10, 15, 0.1) 0%, rgba(8, 10, 15, 0.96) 45%);
}

.actions .row {
  display: flex;
  gap: 0.45rem;
}

.act {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.5em;
  border-radius: 999px;
  transition:
    transform 0.18s var(--ease-out),
    background 0.18s var(--ease-out),
    color 0.18s var(--ease-out);
}

.act:hover {
  transform: translateY(-1px);
}

.act.primary {
  padding: 0.72em 1.1em;
  font-size: 0.86rem;
  font-weight: 600;
  color: #06070b;
  background: var(--accent);
  box-shadow: 0 12px 32px color-mix(in oklab, var(--accent) 34%, transparent);
}

.act.primary:hover {
  background: color-mix(in oklab, var(--accent) 86%, white);
}

.act.primary /* 焦点是画出来的：手柄/键盘两路输入共用同一个下标，所以不用 :focus */
.act.focused {
  color: #fff;
  background: rgba(255, 255, 255, 0.16);
  box-shadow:
    0 0 0 2px color-mix(in oklab, var(--accent) 78%, transparent),
    0 10px 26px rgba(0, 0, 0, 0.35);
}

.act.primary.focused {
  color: #06070b;
  background: color-mix(in oklab, var(--accent) 86%, white);
  box-shadow:
    0 0 0 2px rgba(255, 255, 255, 0.85),
    0 14px 34px color-mix(in oklab, var(--accent) 45%, transparent);
}

.act.ghost.danger-text.focused {
  color: #ffb0b0;
  background: rgba(214, 74, 74, 0.26);
  box-shadow: 0 0 0 2px rgba(224, 90, 90, 0.75);
}

/* ---- 卸载确认 ---- */

.confirm-title {
  margin: 0;
  font-size: 0.9rem;
  font-weight: 600;
}

.confirm-sub {
  margin: 0.3rem 0 0.55rem;
  font-size: 0.72rem;
  line-height: 1.5;
  color: var(--faint);
}

.act.danger {
  flex: 1 1 0;
  padding: 0.6em 0.9em;
  font-size: 0.82rem;
  font-weight: 600;
  color: #fff;
  background: rgba(214, 74, 74, 0.92);
  box-shadow: 0 10px 28px rgba(214, 74, 74, 0.28);
}

.act.danger:hover {
  background: #e05757;
}

.act:disabled {
  opacity: 0.6;
  pointer-events: none;
}

.act.ghost.danger-text {
  color: color-mix(in oklab, #ff7a7a 70%, var(--muted));
}

.act.ghost.danger-text:hover {
  color: #ff9b9b;
  background: rgba(214, 74, 74, 0.22);
}

.glyph {
  color: var(--accent);
  background: rgba(6, 7, 11, 0.8);
}

.act.ghost {
  flex: 1 1 0;
  padding: 0.5em 0.9em;
  font-size: 0.78rem;
  color: var(--muted);
  background: rgba(255, 255, 255, 0.07);
  border: 1px solid rgba(255, 255, 255, 0.09);
}

.act.ghost:hover {
  color: #fff;
  background: rgba(255, 255, 255, 0.13);
}

.act.ghost.on {
  color: var(--accent);
  border-color: color-mix(in oklab, var(--accent) 42%, transparent);
}

/* 焦点是画出来的：手柄/键盘两路输入共用同一个下标，所以不用 :focus */
.act.focused {
  color: #fff;
  background: rgba(255, 255, 255, 0.16);
  box-shadow:
    0 0 0 2px color-mix(in oklab, var(--accent) 78%, transparent),
    0 10px 26px rgba(0, 0, 0, 0.35);
}

.act.primary.focused {
  color: #06070b;
  background: color-mix(in oklab, var(--accent) 86%, white);
  box-shadow:
    0 0 0 2px rgba(255, 255, 255, 0.85),
    0 14px 34px color-mix(in oklab, var(--accent) 45%, transparent);
}

.act.ghost.danger-text.focused {
  color: #ffb0b0;
  background: rgba(214, 74, 74, 0.26);
  box-shadow: 0 0 0 2px rgba(224, 90, 90, 0.75);
}

/* ---- 卸载确认 ---- */

.confirm-title {
  margin: 0;
  font-size: 0.9rem;
  font-weight: 600;
}

.confirm-sub {
  margin: 0.3rem 0 0.55rem;
  font-size: 0.72rem;
  line-height: 1.5;
  color: var(--faint);
}

.act.danger {
  flex: 1 1 0;
  padding: 0.6em 0.9em;
  font-size: 0.82rem;
  font-weight: 600;
  color: #fff;
  background: rgba(214, 74, 74, 0.92);
  box-shadow: 0 10px 28px rgba(214, 74, 74, 0.28);
}

.act.danger:hover {
  background: #e05757;
}

.act:disabled {
  opacity: 0.6;
  pointer-events: none;
}

.act.ghost.danger-text {
  color: color-mix(in oklab, #ff7a7a 70%, var(--muted));
}

.act.ghost.danger-text:hover {
  color: #ff9b9b;
  background: rgba(214, 74, 74, 0.22);
}

.glyph {
  display: inline-grid;
  place-items: center;
  min-width: 1.45em;
  height: 1.45em;
  padding: 0 0.3em;
  border-radius: 999px;
  font-size: 0.7em;
  font-style: normal;
  color: #06070b;
  background: #e9ecf2;
}
</style>
