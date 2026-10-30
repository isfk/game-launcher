<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { Launcher } from "../mygo";
import { sfx } from "../lib/sfx";
import PanelShell from "./PanelShell.vue";
import { usePanelNav } from "../composables/usePanelNav";
import { useLibrary } from "../stores/library";

/**
 * SettingsSheet 是右侧滑出的设置面板。
 *
 * 用抽屉而不是居中弹窗：设置和详情共用一套外壳（PanelShell）和同一套
 * 滑入节奏，切换时不会「一个居中一个靠右」跳来跳去，左侧卡片带也始终可见。
 */
const props = defineProps<{ open?: boolean }>();

const emit = defineEmits<{
  close: [];
  rescan: [];
  toggleFullscreen: [];
  /** 手动添加成功（外层弹提示） */
  added: [];
  /** 添加失败（外层弹错误） */
  failed: [message: string];
}>();

const lib = useLibrary();

const fullScreen = ref(false);
const pads = ref<string[]>([]);
// 音效开关的唯一真相在后端的 settings.json，这里直接读 store 里的镜像
const sound = computed(() => lib.soundEnabled);

/** 每次展开都重新问一遍后端：全屏状态可能被系统或快捷键改掉。 */
async function refresh() {
  try {
    fullScreen.value = await Launcher.isFullScreen();
    pads.value = await Launcher.padNames();
  } catch {
    // 后端不可用时保持默认值，设置面板不该因为这些挂掉
  }
}

onMounted(refresh);
watch(() => props.open, (open) => open && void refresh());

/** 键盘捷径一览。写死在组件里而不是读后端：这类文案跟着 UI 一起改。 */
const shortcuts = [
  ["↑ ↓", "选择项"],
  ["← →", "切换游戏"],
  ["Enter", "启动"],
  ["↑", "详情"],
  ["X", "收藏"],
  ["S", "设置"],
  ["/", "搜索"],
  ["F", "全屏"],
  ["R", "重新扫描"],
  ["Esc", "返回"],
];

const nav = usePanelNav(() => [
  { key: "fullscreen", activate: () => emit("toggleFullscreen") },
  { key: "add", activate: () => void addGame() },
  { key: "rescan", activate: () => emit("rescan") },
  { key: "iconpos", activate: () => void cycleIcon() },
  { key: "sound", activate: () => toggleSound() },
  { key: "quit", activate: () => void Launcher.quit() },
]);

/** 位置的显示文案：就是四个词，和卡片上的四个角一一对应。 */
const ICON_POS_LABEL: Record<string, string> = {
  tl: "左上",
  tr: "右上",
  bl: "左下",
  br: "右下",
};
const iconPosLabel = computed(() => ICON_POS_LABEL[lib.iconPos] ?? "左下");

/** cycleIcon 依次切换四个角，切一下给个短反馈。 */
function cycleIcon() {
  sfx.move();
  void lib.cycleIconPos();
}

defineExpose({ move: nav.move, activate: nav.activate, reset: nav.reset, isFocused: nav.isFocused });

const adding = ref(false);

/** addGame 走系统文件选择框；加完让外层的库刷新一次（后端已经重扫）。 */
async function addGame() {
  if (adding.value) return;
  adding.value = true;
  try {
    if (await lib.addManualGame()) {
      emit("added");
      sfx.confirm();
    }
  } catch (e) {
    emit("failed", e instanceof Error ? e.message : String(e));
  } finally {
    adding.value = false;
  }
}

function toggleSound() {
  const next = !lib.soundEnabled;
  // 先切实际发声（store 落盘是异步的），开着的话顺带给一次听觉反馈
  sfx.setEnabled(next);
  if (next) sfx.confirm();
  void lib.setSoundEnabled(next);
}
</script>

<template>
  <PanelShell :open="open" accent="#d8a92a" label="设置">
    <header class="head">
      <div>
        <h2>设置</h2>
        <p class="sub">Esc / ○ 返回</p>
      </div>
      <button class="x" aria-label="关闭" @click="$emit('close')">✕</button>
    </header>

    <div class="body">
      <ul class="rows">
        <li>
          <div>
            <span class="label">全屏</span>
            <span class="desc">{{ fullScreen ? "已全屏（隐藏桌面任务栏 / 菜单栏）" : "窗口模式" }}</span>
          </div>
          <button
            class="btn"
            :class="{ focused: nav.isFocused('fullscreen') }"
            @mouseenter="nav.focusTo(0)"
            @click="$emit('toggleFullscreen')"
          >
            切换
          </button>
        </li>

        <li>
          <div>
            <span class="label">添加游戏</span>
            <span class="desc">{{ adding ? "正在等文件选择框…" : "手动挑一个 .app / .exe 加进库里" }}</span>
          </div>
          <button
            class="btn"
            :class="{ focused: nav.isFocused('add') }"
            :disabled="adding"
            @mouseenter="nav.focusTo(1)"
            @click="addGame"
          >
            {{ adding ? "…" : "添加" }}
          </button>
        </li>

        <li>
          <div>
            <span class="label">重新扫描游戏库</span>
            <span class="desc">重新读取 Steam 的 libraryfolders.vdf 与 appmanifest</span>
          </div>
          <button class="btn" :class="{ focused: nav.isFocused('rescan') }" @mouseenter="nav.focusTo(2)" @click="$emit('rescan')">
            扫描
          </button>
        </li>

        <li>
          <div>
            <span class="label">平台图标位置</span>
            <span class="desc">点击或 Enter 切换</span>
          </div>
          <button
            class="btn"
            :class="{ focused: nav.isFocused('iconpos') }"
            @mouseenter="nav.focusTo(3)"
            @click="cycleIcon"
          >
            {{ iconPosLabel }}
          </button>
        </li>

        <li>
          <div>
            <span class="label">界面音效</span>
            <span class="desc">{{ sound ? "开" : "关" }}</span>
          </div>
          <button class="btn" :class="{ focused: nav.isFocused('sound') }" @mouseenter="nav.focusTo(4)" @click="toggleSound">
            {{ sound ? "关闭" : "打开" }}
          </button>
        </li>

        <li>
          <div>
            <span class="label">手柄</span>
            <span class="desc">{{ pads.length ? pads.join(" · ") : "未检测到手柄（键盘仍可用）" }}</span>
          </div>
        </li>

        <li>
          <div>
            <span class="label">退出启动器</span>
            <span class="desc">关闭窗口，不影响已启动的游戏</span>
          </div>
          <button
            class="btn danger"
            :class="{ focused: nav.isFocused('quit') }"
            @mouseenter="nav.focusTo(5)"
            @click="Launcher.quit()"
          >
            退出
          </button>
        </li>
      </ul>
    </div>

    <!-- 底部键位速查：手柄玩家也用得上，顺便把面板下半截填满 -->
    <footer class="foot">
      <p class="foot-title">键盘捷径</p>
      <ul class="keys">
        <li v-for="item in shortcuts" :key="item[0]">
          <kbd>{{ item[0] }}</kbd>
          <span>{{ item[1] }}</span>
        </li>
      </ul>
    </footer>
  </PanelShell>
</template>

<style scoped>
.head {
  flex: 0 0 auto;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
  padding: calc(6vh + 0.5rem) 1.3rem 1rem;
}

h2 {
  margin: 0;
  font-size: 1.5rem;
  letter-spacing: -0.01em;
}

.sub {
  margin: 0.3rem 0 0;
  font-size: 0.78rem;
  color: var(--faint);
}

.x {
  flex: 0 0 auto;
  display: grid;
  place-items: center;
  width: 2rem;
  height: 2rem;
  border-radius: 50%;
  font-size: 0.8rem;
  color: var(--muted);
  background: rgba(255, 255, 255, 0.07);
  transition:
    color 0.2s var(--ease-out),
    background 0.2s var(--ease-out);
}

.x:hover {
  color: #fff;
  background: rgba(255, 255, 255, 0.16);
}

.body {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
  padding: 0 1.3rem 1rem;
}

.rows {
  list-style: none;
  margin: 0;
  padding: 0;
}

.rows li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1.2rem;
  padding: 0.95rem 0;
  border-top: 1px solid rgba(255, 255, 255, 0.07);
}

.rows li:first-child {
  border-top: 0;
  padding-top: 0.2rem;
}

.foot {
  flex: 0 0 auto;
  padding: 0.9rem 1.3rem calc(6vh + 0.6rem);
  border-top: 1px solid rgba(255, 255, 255, 0.07);
  background: linear-gradient(180deg, rgba(8, 10, 15, 0.2) 0%, rgba(8, 10, 15, 0.95) 55%);
}

.foot-title {
  margin: 0 0 0.5rem;
  font-size: 0.68rem;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--faint);
}

.keys {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.4rem 0.9rem;
  margin: 0;
  padding: 0;
  list-style: none;
  font-size: 0.74rem;
  color: var(--muted);
}

.keys li {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

kbd {
  flex: 0 0 auto;
  min-width: 3.1rem;
  padding: 0.15em 0.45em;
  border-radius: 6px;
  font-family: inherit;
  font-size: 0.68rem;
  text-align: center;
  color: var(--fg);
  background: rgba(255, 255, 255, 0.08);
  border: 1px solid rgba(255, 255, 255, 0.1);
}

.label {
  display: block;
  font-size: 0.9rem;
  font-weight: 550;
}

.desc {
  display: block;
  margin-top: 0.2rem;
  font-size: 0.76rem;
  color: var(--faint);
  overflow-wrap: anywhere;
}

.btn {
  flex: 0 0 auto;
  padding: 0.5em 1.2em;
  border-radius: 999px;
  font-size: 0.82rem;
  background: rgba(255, 255, 255, 0.1);
  transition: background 0.2s var(--ease-out);
}

.btn:hover {
  background: rgba(255, 255, 255, 0.18);
}

/* 焦点是画出来的（手柄/键盘共用同一个下标） */
.btn.focused {
  color: #fff;
  background: rgba(255, 255, 255, 0.2);
  box-shadow: 0 0 0 2px color-mix(in oklab, var(--accent) 72%, transparent);
}

.btn.danger.focused {
  background: rgba(214, 74, 74, 0.5);
  box-shadow: 0 0 0 2px rgba(224, 90, 90, 0.7);
}

.btn.danger:hover {
  background: rgba(224, 90, 90, 0.32);
}
</style>
