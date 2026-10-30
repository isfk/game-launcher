import { computed, ref } from "vue";
import { defineStore } from "pinia";
import { isMyGo } from "mygo-runtime";
import { Launcher, type ArtworkUpdate, type Game } from "../mygo";

const FAVORITES_KEY = "launcher.favorites";

/** useLibrary 是界面唯一的数据源：游戏列表、当前焦点、扫描状态。 */
export const useLibrary = defineStore("library", () => {
  const games = ref<Game[]>([]);
  const source = ref(""); // steam | demo
  const roots = ref<string[]>([]);
  const index = ref(0);
  const loading = ref(false);
  const error = ref("");
  const favorites = ref<Set<string>>(loadFavorites());
  // 界面音效开关存在后端（settings.json），这里只是镜像
  const soundEnabled = ref(true);
  // 卡片上平台图标的位置：tl / tr / bl（默认，和游戏名同一行）/ br
  const iconPos = ref<string>("bl");
  // 已经问过后端要过主色的 key（含没取到的情况），避免反复请求
  const accentAsked = new Set<string>();

  const selected = computed<Game | undefined>(() => games.value[index.value]);
  const isDemo = computed(() => source.value === "demo");

  /**
   * loadSettings 从后端读一次用户设置（音效开关、收藏）。
   *
   * 浏览器里没有后端，就沿用 localStorage 里的收藏；真机这边后端是唯一真相，
   * 免得出现「界面记得、后端不知道」的两份数据。
   */
  async function loadSettings() {
    if (!isMyGo()) return;
    try {
      const s = await Launcher.settings();
      soundEnabled.value = s.soundEnabled;
      iconPos.value = s.iconPos || "bl";
      favorites.value = new Set(s.favorites ?? []);
    } catch {
      // 读不到设置就按默认值走，不影响使用
    }
  }

  /** 四个位置按顺序轮转，设置面板里一个按钮就能切。 */
  const ICON_POSITIONS = ["bl", "tl", "tr", "br"] as const;
  type IconPos = (typeof ICON_POSITIONS)[number];

  /** cycleIconPos 把图标位置切到下一个，并落盘。 */
  async function cycleIconPos() {
    const i = ICON_POSITIONS.indexOf(iconPos.value as IconPos);
    const next: IconPos = ICON_POSITIONS[(i + 1) % ICON_POSITIONS.length] ?? "bl";
    iconPos.value = next;
    if (!isMyGo()) return;
    try {
      const s = await Launcher.setIconPos(next);
      iconPos.value = s.iconPos || next;
    } catch {
      // 落盘失败先用内存里的值
    }
  }

  /** setSoundEnabled 改音效开关并落盘。 */
  async function setSoundEnabled(on: boolean) {
    soundEnabled.value = on;
    if (!isMyGo()) return;
    try {
      const s = await Launcher.setSoundEnabled(on);
      soundEnabled.value = s.soundEnabled;
      iconPos.value = s.iconPos || "bl";
    } catch {
      // 落盘失败就先用着内存里的值
    }
  }

  /** load 拉一次游戏库。refresh 为真时让后端重新扫盘。 */
  async function load(refresh = false) {
    loading.value = true;
    error.value = "";
    try {
      // 在普通浏览器里打开 dev server 时没有 Go 后端，用本地样例数据，
      // 这样调样式和排版不用每次都开应用。
      const result = isMyGo() ? await Launcher.library(refresh) : { games: browserDemo(), source: "demo", roots: [] };
      apply(result.games, result.source, result.roots);
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
    } finally {
      loading.value = false;
    }
  }

  function apply(games_: Game[], source_: string, roots_: string[]) {
    accentAsked.clear();
    // 重扫出来的对象是全新的：把上一批里已经取到的主色和运行状态搬过来，
    // 否则后台每发现一次库变化，界面上的主色就会闪一下、运行中标记也会丢。
    //
    // 注意这里是构造新对象而不是就地赋值：后台事件送来的载荷是冻结的，
    // 直接写 game.accent 会抛 TypeError（严格模式下），列表就换不掉了。
    const before = new Map(games.value.map((g) => [g.key, g]));
    const keepKey = selected.value?.key;

    games.value = withFavoritesFirst(
      games_.map((game) => {
        const old = before.get(game.key);
        if (!old) return game;
        return { ...game, accent: game.accent || old.accent, running: game.running || old.running };
      }),
    );
    source.value = source_;
    roots.value = roots_;
    // 刷新后尽量停在原来那部游戏上，别把用户甩回第一个。
    const found = keepKey ? games.value.findIndex((g) => g.key === keepKey) : -1;
    index.value = found >= 0 ? found : 0;
  }

  /** withFavoritesFirst 收藏置顶，其余保持后端给的顺序。 */
  function withFavoritesFirst(list: Game[]): Game[] {
    if (!favorites.value.size) return list;
    return [...list].sort((a, b) => {
      const fa = favorites.value.has(a.key) ? 0 : 1;
      const fb = favorites.value.has(b.key) ? 0 : 1;
      return fa - fb;
    });
  }

  /**
   * sortGames 收藏变了之后重排，并把焦点跟回原来那部游戏。
   *
   * 为什么排序而不是渲染时排：卡片下标就是焦点下标，
   * 两边顺序不一致的话 ←→ 会跳着走。
   */
  function sortGames() {
    if (!favorites.value.size) return;
    const keepKey = selected.value?.key;
    games.value = withFavoritesFirst(games.value);
    if (keepKey) focusKey(keepKey);
  }

  /** focusKey 把焦点直接放到某部游戏上（详情面板用）。 */
  function focusKey(key: string) {
    const i = games.value.findIndex((g) => g.key === key);
    if (i >= 0) index.value = i;
  }

  /** addManualGame 弹系统文件选择框，加进来的游戏会成为新的一张卡。 */
  async function addManualGame(): Promise<boolean> {
    if (!isMyGo()) return false;
    const result = await Launcher.addManualGame();
    // 用户取消时后端原样返回旧库，数量没变就不用重排
    if (result.games.length === games.value.length) return false;
    apply(result.games, result.source, result.roots);
    return true;
  }

  /** removeManualGame 把手动添加的游戏从库里移除（不动磁盘上的文件）。 */
  async function removeManualGame(key: string): Promise<void> {
    if (!isMyGo()) return;
    const result = await Launcher.removeManualGame(key);
    apply(result.games, result.source, result.roots);
  }

  /** move 左右移动焦点，到头会绕回另一端。返回是否真的移动了。 */
  function move(delta: number): boolean {
    const n = games.value.length;
    if (!n) return false;
    const next = (index.value + delta + n) % n;
    if (next === index.value) return false;
    index.value = next;
    return true;
  }

  /**
   * ensureAccent 按需向后端要一部游戏的主色（从封面提取），并写回列表。
   * 没封面 / 取不到就保持空，界面会退回按标题散列的颜色。
   */
  async function ensureAccent(key?: string) {
    const target = key ?? selected.value?.key;
    if (!target || accentAsked.has(target)) return;

    const game = games.value.find((g) => g.key === target);
    if (!game || game.accent) return;

    accentAsked.add(target);
    if (!isMyGo()) return;

    try {
      const accent = await Launcher.accent(target);
      if (accent) game.accent = accent;
    } catch {
      // 取不到主色不影响任何功能
    }
  }

  /** applyArtwork 把后端缓存到本地的图片与主色写回列表。 */
  function applyArtwork(update: ArtworkUpdate) {
    const game = games.value.find((g) => g.key === update.key);
    if (!game) return;
    game.artwork = update.artwork;
    if (update.accent) game.accent = update.accent;
  }

  /** setRunning 更新「是否正在运行」，卡片上会显示小圆点。 */
  function setRunning(key: string, running: boolean) {
    const game = games.value.find((g) => g.key === key);
    if (game) game.running = running;
  }

  /** isFavorite 判断某部游戏是否已收藏（详情页要用）。 */
  function isFavorite(key?: string): boolean {
    return !!key && favorites.value.has(key);
  }

  function toggleFavorite(key?: string) {
    const target = key ?? selected.value?.key;
    if (!target) return false;

    const next = new Set(favorites.value);
    const added = !next.has(target);
    if (added) next.add(target);
    else next.delete(target);
    // 先乐观更新界面，再异步落盘：点收藏不该等 IO
    favorites.value = next;

    if (isMyGo()) {
      void Launcher.setFavorite(target, added)
        .then((s) => {
          favorites.value = new Set(s.favorites ?? []);
        })
        .catch(() => {
          // 落盘失败：界面保持乐观结果，下次启动会回到磁盘上的状态
        });
    } else {
      localStorage.setItem(FAVORITES_KEY, JSON.stringify([...next]));
    }
    sortGames();
    return added;
  }

  return {
    games,
    source,
    roots,
    index,
    loading,
    error,
    favorites,
    soundEnabled,
    iconPos,
    cycleIconPos,
    selected,
    isDemo,
    load,
    loadSettings,
    addManualGame,
    removeManualGame,
    setSoundEnabled,
    apply,
    move,
    ensureAccent,
    applyArtwork,
    setRunning,
    focusKey,
    isFavorite,
    toggleFavorite,
  };
});

function loadFavorites(): Set<string> {
  try {
    const raw = localStorage.getItem(FAVORITES_KEY);
    const parsed: unknown = raw ? JSON.parse(raw) : [];
    return new Set(Array.isArray(parsed) ? parsed.filter((k): k is string => typeof k === "string") : []);
  } catch {
    return new Set();
  }
}

/**
 * browserDemo 是「在浏览器里直接打开 dev server」时用的样例数据。
 * 真实运行时数据全部来自 Go（Steam 扫描），这里只为了让调样式不用开应用。
 */
function browserDemo(): Game[] {
  const demo: Array<[string, string]> = [
    ["艾尔登法环", "#c9a227"],
    ["赛博朋克 2077", "#e0c341"],
    ["哈迪斯 II", "#c0392b"],
    ["博德之门 3", "#a5642f"],
    ["空洞骑士：丝之歌", "#2f8fb0"],
    ["极乐迪斯科", "#5a6a8a"],
    ["星露谷物语", "#5aa02f"],
  ];
  return demo.map(([title, accent], i) => ({
    key: `demo:${i + 1}`,
    title,
    provider: "demo",
    appId: String(100000 + i),
    artwork: {},
    accent,
    installed: true,
    running: false,
    playtimeMinutes: 300 + i * 240,
    lastPlayed: new Date(Date.now() - (i + 1) * 86_400_000).toISOString(),
  }));
}
