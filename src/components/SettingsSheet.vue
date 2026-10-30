<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue";
import { Launcher } from "../mygo";
import { providerLabel } from "../lib/format";
import { sfx } from "../lib/sfx";
import { glyphsFor } from "../lib/glyphs";
import { padKind, usingPad } from "../lib/input";
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
  ["Enter", "启动 / 确认"],
  // 详情认 Y（对位手柄 Y / △，全部游戏里也是 Y），首页额外送一个 ↑
  ["Y / ↑", "详情"],
  ["X", "收藏"],
  ["S", "设置"],
  ["/", "搜索"],
  ["G", "全部游戏 / 关闭"],
  ["F", "全屏"],
  ["R", "重新扫描"],
  ["Esc", "返回 / 关闭"],
];

const nav = usePanelNav(() => [
  // 资料：昵称和头像。放在最上面 —— 右上角那个圆点就是点这里进去的。
  { key: "name", activate: () => void startNameEdit() },
  { key: "avatar", activate: () => void pickAvatar() },
  ...(lib.profileAvatar ? [{ key: "avatar-clear", activate: () => void clearAvatar() }] : []),
  { key: "fullscreen", activate: () => emit("toggleFullscreen") },
  { key: "add", activate: () => void addGame() },
  { key: "rescan", activate: () => emit("rescan") },
  { key: "iconpos", activate: () => void cycleIcon() },
  { key: "unhide", activate: () => void lib.unhideAll() },
  // 每个被隐藏的游戏单独一条：可以只恢复其中一部
  ...lib.hiddenGames.map((g) => ({ key: `unhide:${g.key}`, activate: () => void lib.toggleHidden(g.key, false) })),
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

/**
 * page 肩键翻页：LB / RB 一次走 5 行。
 *
 * 设置项是竖着一长条，肩键在主机上是「换一屏内容」的位置；
 * 这里没有分页签，就按屏来 —— 比一格一格按十几次像样。
 * 鼠标不受影响：行照样可点、可悬停聚焦。
 */
/** 关闭按钮：手柄写返回键（B / ○ / A），键鼠写 ✕ */
const closeCap = computed(() => (usingPad.value ? glyphsFor(padKind.value).back.text : "✕"));

function page(dir: number) {
  return nav.move(dir * 5);
}

defineExpose({
  move: nav.move,
  page,
  activate: nav.activate,
  reset: nav.reset,
  isFocused: nav.isFocused,
});

/**
 * focusTo 按 key 找下标再移动鼠标焦点。
 *
 * 不写死数字：设置项会随状态增删（隐藏列表就是动态的），
 * 硬编码下标一旦对不上，鼠标悬停就会把焦点挪到别的行上去。
 */
function focusTo(key: string) {
  const i = nav.list.value.findIndex((item) => item.key === key);
  if (i >= 0) nav.focusTo(i);
}

const adding = ref(false);

// ---- 资料：昵称与头像 ----

// 昵称改成「就地进输入框」：不在弹窗里再弹一个输入框，
// 设置面板本身的遮罩已经够多一层了。
const editingName = ref(false);
const draftName = ref("");
const nameEl = ref<HTMLInputElement | null>(null);

async function startNameEdit() {
  sfx.move();
  draftName.value = lib.profileName;
  editingName.value = true;
  await nextTick();
  nameEl.value?.focus();
  nameEl.value?.select();
}

/** 关掉输入态（不落盘）。先改状态再失焦，避免 blur 把取消又当成保存。 */
function cancelNameEdit() {
  editingName.value = false;
}

/** 失焦 / Enter 都算保存；内容没变就不发请求。 */
function commitNameEdit() {
  if (!editingName.value) return;
  const next = draftName.value.trim();
  editingName.value = false;
  if (next === lib.profileName) return;
  void lib
    .setNickname(next)
    .then(() => sfx.confirm())
    .catch((e) => emit("failed", e instanceof Error ? e.message : String(e)));
}

function onNameKey(e: KeyboardEvent) {
  if (e.key === "Enter") {
    e.preventDefault();
    commitNameEdit();
  } else if (e.key === "Escape") {
    e.preventDefault();
    cancelNameEdit();
    nameEl.value?.blur();
  }
}

/** pickAvatar 走系统文件选择框；取消就什么都不变，失败要把原因说出来。 */
async function pickAvatar() {
  try {
    await lib.pickAvatar();
    sfx.confirm();
  } catch (e) {
    emit("failed", e instanceof Error ? e.message : String(e));
  }
}

async function clearAvatar() {
  try {
    await lib.clearAvatar();
    sfx.back();
  } catch (e) {
    emit("failed", e instanceof Error ? e.message : String(e));
  }
}


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
      <button class="x" aria-label="关闭" :title="`关闭（${closeCap}）`" @click="$emit('close')">
        {{ closeCap }}
      </button>
    </header>

    <div class="body">
      <ul class="rows">
        <!-- 资料：右上角那个头像点进来就是这里 -->
        <li class="profile">
          <div class="profile-main">
            <span class="avatar-sm">
              <img v-if="lib.profileAvatar" :src="lib.profileAvatar" alt="" />
              <template v-else>{{ lib.profileInitial }}</template>
            </span>
            <div>
              <span class="label">昵称</span>
              <span class="desc">
                {{ editingName ? "输入完按 Enter 保存，Esc 取消" : lib.profileName }}
              </span>
            </div>
          </div>
          <input
            v-if="editingName"
            ref="nameEl"
            v-model="draftName"
            class="name-input"
            type="text"
            :maxlength="24"
            placeholder="留空则用系统账户名"
            spellcheck="false"
            autocomplete="off"
            @keydown.stop="onNameKey"
            @blur="commitNameEdit"
          />
          <button
            v-else
            class="btn"
            :class="{ focused: nav.isFocused('name') }"
            @mouseenter="focusTo('name')"
            @click="startNameEdit"
          >
            修改
          </button>
        </li>

        <li>
          <div>
            <span class="label">头像</span>
            <span class="desc">
              {{ lib.profileAvatar ? "已使用自定义图片" : "默认用昵称首字 + 渐变" }}
            </span>
          </div>
          <div class="btns">
            <button
              class="btn"
              :class="{ focused: nav.isFocused('avatar') }"
              @mouseenter="focusTo('avatar')"
              @click="pickAvatar"
            >
              选择图片
            </button>
            <button
              v-if="lib.profileAvatar"
              class="btn"
              :class="{ focused: nav.isFocused('avatar-clear') }"
              @mouseenter="focusTo('avatar-clear')"
              @click="clearAvatar"
            >
              清除
            </button>
          </div>
        </li>

        <li>
          <div>
            <span class="label">全屏</span>
            <span class="desc">{{ fullScreen ? "已全屏（隐藏桌面任务栏 / 菜单栏）" : "窗口模式" }}</span>
          </div>
          <button
            class="btn"
            :class="{ focused: nav.isFocused('fullscreen') }"
            @mouseenter="focusTo('fullscreen')"
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
            @mouseenter="focusTo('add')"
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
          <button class="btn" :class="{ focused: nav.isFocused('rescan') }" @mouseenter="focusTo('rescan')" @click="$emit('rescan')">
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
            @mouseenter="focusTo('iconpos')"
            @click="cycleIcon"
          >
            {{ iconPosLabel }}
          </button>
        </li>

        <li>
          <div>
            <span class="label">隐藏的游戏</span>
            <span class="desc">
              {{ lib.hiddenCount ? `共 ${lib.hiddenCount} 部（客户端没动，随时可恢复）` : "没有被隐藏的游戏" }}
            </span>
          </div>
          <button
            class="btn"
            :class="{ focused: nav.isFocused('unhide') }"
            :disabled="!lib.hiddenCount"
            @mouseenter="focusTo('unhide')"
            @click="lib.unhideAll()"
          >
            {{ lib.hiddenCount ? "全部恢复" : "无" }}
          </button>
        </li>

        <!-- 每个隐藏的游戏单独一行，只想放回某一部就点它自己的「恢复」 -->
        <li v-for="g in lib.hiddenGames" :key="g.key" class="hidden-game">
          <div>
            <span class="label">{{ g.title }}</span>
            <span class="desc">{{ providerLabel(g.provider) }} · 已隐藏</span>
          </div>
          <button
            class="btn"
            :class="{ focused: nav.isFocused(`unhide:${g.key}`) }"
            @mouseenter="focusTo(`unhide:${g.key}`)"
            @click="lib.toggleHidden(g.key, false)"
          >
            恢复
          </button>
        </li>

        <li>
          <div>
            <span class="label">界面音效</span>
            <span class="desc">{{ sound ? "开" : "关" }}</span>
          </div>
          <button class="btn" :class="{ focused: nav.isFocused('sound') }" @mouseenter="focusTo('sound')" @click="toggleSound">
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
            @mouseenter="focusTo('quit')"
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
  background: var(--capsule-bg);
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

/* ---- 资料 ---- */
.profile-main {
  display: flex;
  align-items: center;
  gap: 0.7rem;
  min-width: 0;
}

.avatar-sm {
  flex: 0 0 auto;
  display: grid;
  place-items: center;
  width: 2.1rem;
  height: 2.1rem;
  border-radius: 50%;
  overflow: hidden;
  font-size: 0.9rem;
  font-weight: 700;
  color: #fff;
  background: linear-gradient(140deg, #4a6cd4, #8a4ad4);
  box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.14);
}

.avatar-sm img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.name-input {
  flex: 0 0 auto;
  width: 9rem;
  padding: 0.4em 0.7em;
  border-radius: 10px;
  border: 1px solid rgba(255, 255, 255, 0.16);
  background: rgba(0, 0, 0, 0.35);
  color: var(--fg);
  font-size: 0.85rem;
  font-family: inherit;
  outline: none;
}

.name-input:focus {
  border-color: color-mix(in oklab, var(--accent) 72%, transparent);
}

.btns {
  display: flex;
  align-items: center;
  gap: 0.5rem;
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
