import { computed, ref } from "vue";
import { defineStore } from "pinia";
import { isMyGo } from "mygo-runtime";
import { Launcher, type ArtworkUpdate, type Game } from "../mygo";

const FAVORITES_KEY = "launcher.favorites";
// 浏览器里没有后端（dev server 调样式时），昵称就存在本地。
const NAME_KEY = "launcher.name";

/** useLibrary 是界面唯一的数据源：游戏列表、当前焦点、扫描状态。 */
export const useLibrary = defineStore("library", () => {
  // allGames 是后端给的完整列表；games 是「过滤 + 排序」之后真正显示的那份。
  // 分开存是为了让隐藏、筛选、排序都只是视图层的事：数据不动，
  // 取消隐藏 / 换筛选时立刻能还原回来。
  const allGames = ref<Game[]>([]);
  const hidden = ref<Set<string>>(new Set());
  const sortBy = ref<string>("recent");
  const sortDir = ref<string>("desc");
  const platformFilter = ref<string>(""); // "" = 全部平台
  // 自定义顺序（全部游戏里「调整顺序」写下的）。sortBy 为 manual 时生效。
  const order = ref<string[]>([]);
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
  // 右上角「我是谁」：昵称与头像。头像为空串时界面用昵称首字 + 渐变兜底。
  // 昵称的真相在后端（没自定义就是系统账户名），这里只是镜像。
  const profileName = ref<string>("");
  const profileAvatar = ref<string>("");
  const profileInitial = computed(() => {
    const first = [...profileName.value.trim()][0] ?? "?";
    return first.toUpperCase();
  });
  // 已经问过后端要过主色的 key（含没取到的情况），避免反复请求
  const accentAsked = new Set<string>();

  /** games 是视图列表：去掉隐藏的、按平台筛选、再排序（收藏永远置顶）。 */
  const games = computed<Game[]>(() => {
    const list = allGames.value.filter(
      (g) => !hidden.value.has(g.key) && (!platformFilter.value || g.provider === platformFilter.value),
    );
    return orderGames(list);
  });

  const selected = computed<Game | undefined>(() => games.value[index.value]);
  const isDemo = computed(() => source.value === "demo");
  const hiddenCount = computed(() => hidden.value.size);
  /** 被隐藏的游戏本体（设置面板要逐条列出、逐条恢复）。 */
  const hiddenGames = computed(() => allGames.value.filter((g) => hidden.value.has(g.key)));

  /**
   * loadSettings 从后端读一次用户设置（音效开关、收藏、昵称头像）。
   *
   * 浏览器里没有后端，昵称就沿用 localStorage；真机这边后端是唯一真相，
   * 免得出现「界面记得、后端不知道」的两份数据。
   */
  async function loadSettings() {
    if (!isMyGo()) {
      // 调样式时的兜底：host 浏览器里没有账户名，就叫「玩家」
      profileName.value = localStorage.getItem(NAME_KEY) ?? "玩家";
      return;
    }
    try {
      const [s, p] = await Promise.all([Launcher.settings(), Launcher.profile()]);
      soundEnabled.value = s.soundEnabled;
      iconPos.value = s.iconPos || "bl";
      hidden.value = new Set(s.hidden ?? []);
      sortBy.value = s.sortBy || "recent";
      sortDir.value = s.sortDir || "desc";
      order.value = s.order ?? [];
      favorites.value = new Set(s.favorites ?? []);
      applyProfile(p);
    } catch {
      // 读不到设置就按默认值走，不影响使用
    }
  }

  function applyProfile(p: { name: string; avatar: string }) {
    profileName.value = p.name;
    profileAvatar.value = p.avatar;
  }

  /** setNickname 记住昵称，空串表示回到系统账户名。 */
  async function setNickname(name: string) {
    if (!isMyGo()) {
      profileName.value = name.trim();
      localStorage.setItem(NAME_KEY, profileName.value);
      return;
    }
    applyProfile(await Launcher.setNickname(name));
  }

  /** pickAvatar 弹系统文件选择框换头像（取消就什么都不变）。 */
  async function pickAvatar() {
    if (!isMyGo()) return;
    applyProfile(await Launcher.pickAvatar());
  }

  /** clearAvatar 回到「昵称首字 + 渐变」的兜底样子。 */
  async function clearAvatar() {
    if (!isMyGo()) return;
    applyProfile(await Launcher.clearAvatar());
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

  /** 排序字段的轮转顺序，和设置面板里看到的一致。 */
  const SORT_OPTIONS = ["recent", "name", "playtime", "size"] as const;
  /** 平台筛选的轮转顺序：全部 → 各平台 → 全部。 */
  const FILTER_OPTIONS = ["", "steam", "epic", "wegame", "manual"] as const;

  /** toggleHidden 只在启动器里藏 / 恢复一部游戏（不动客户端、不动磁盘）。 */
  async function toggleHidden(key: string, hide: boolean) {
    const keepKey = selected.value?.key;
    const next = new Set(hidden.value);
    if (hide) next.add(key);
    else next.delete(key);
    hidden.value = next; // 先乐观更新，界面立刻反应

    // 藏掉的正好是当前这部：焦点往下顺延一位
    reconcileFocus(hide && keepKey === key ? undefined : keepKey);

    if (!isMyGo()) return;
    try {
      const s = await Launcher.setHidden(key, hide);
      hidden.value = new Set(s.hidden ?? []);
    } catch {
      // 落盘失败就先用内存里的值
    }
  }

  /** unhideAll 一次性把藏起来的都恢复。 */
  async function unhideAll() {
    const keys = [...hidden.value];
    hidden.value = new Set();
    if (!isMyGo()) return;
    for (const key of keys) {
      try {
        await Launcher.setHidden(key, false);
      } catch {
        // 单条失败不影响其他
      }
    }
  }

  /** cycleSortBy 换一个排序字段并落盘。
   *
   * 调过自定义顺序的话，「自定义」也占一档：玩家能从它转出去，也能转回来。
   */
  async function cycleSortBy() {
    const options: string[] = order.value.length ? [...SORT_OPTIONS, "manual"] : [...SORT_OPTIONS];
    const i = options.indexOf(sortBy.value);
    const next = options[(i + 1) % options.length] ?? "recent";
    const keepKey = selected.value?.key;
    sortBy.value = next;
    reconcileFocus(keepKey);
    if (!isMyGo()) return;
    try {
      const s = await Launcher.setSort(next, sortDir.value);
      sortBy.value = s.sortBy || next;
    } catch {
      // 落盘失败先用内存值
    }
  }

  /**
   * setManualOrder 把「界面上看到的顺序」写成自定义顺序。
   *
   * visibleKeys 只覆盖当前筛选下的那些卡。被筛掉、被隐藏的游戏不该因为
   * 一次拖动就丢掉位置，所以做法是：取当前排序下的全表，把可见的那几个槽位
   * 按新顺序重新填进去，其余位置原样不动。
   */
  async function setManualOrder(visibleKeys: string[]) {
    const keepKey = selected.value?.key;
    const visible = new Set(visibleKeys);
    const current = orderGames(allGames.value);

    const slots: number[] = [];
    current.forEach((g, i) => {
      if (visible.has(g.key)) slots.push(i);
    });

    const next = [...current];
    const wanted = visibleKeys
      .map((key) => current.find((g) => g.key === key))
      .filter((g): g is Game => g !== undefined);
    slots.forEach((slot, i) => {
      const game = wanted[i];
      if (game) next[slot] = game;
    });

    order.value = next.map((g) => g.key);
    sortBy.value = "manual";
    reconcileFocus(keepKey);

    if (!isMyGo()) return;
    try {
      const s = await Launcher.setOrder(order.value);
      order.value = s.order ?? order.value;
      sortBy.value = s.sortBy || "manual";
      reconcileFocus(keepKey);
    } catch {
      // 落盘失败就先用内存里的顺序，下次启动回到磁盘上的状态
    }
  }

  /** toggleSortDir 切换升 / 降序并落盘。 */
  async function toggleSortDir() {
    const next = sortDir.value === "desc" ? "asc" : "desc";
    const keepKey = selected.value?.key;
    sortDir.value = next;
    reconcileFocus(keepKey);
    if (!isMyGo()) return;
    try {
      const s = await Launcher.setSort(sortBy.value, next);
      sortDir.value = s.sortDir || next;
    } catch {
      // 落盘失败先用内存值
    }
  }

  /** cyclePlatformFilter 在「全部 → Steam → Epic → WeGame → 手动」之间轮转。 */
  function cyclePlatformFilter() {
    const i = FILTER_OPTIONS.indexOf(platformFilter.value as (typeof FILTER_OPTIONS)[number]);
    const next = FILTER_OPTIONS[(i + 1) % FILTER_OPTIONS.length] ?? "";
    platformFilter.value = next;
    index.value = 0; // 换了筛选，回到第一部
  }

  /** setPlatformFilter 直接指定筛选（顶栏点平台用）。 */
  function setPlatformFilter(id: string) {
    platformFilter.value = platformFilter.value === id ? "" : id;
    index.value = 0;
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
    const before = new Map(allGames.value.map((g) => [g.key, g]));
    const keepKey = selected.value?.key;

    allGames.value = games_.map((game) => {
      const old = before.get(game.key);
      if (!old) return game;
      return { ...game, accent: game.accent || old.accent, running: game.running || old.running };
    });
    source.value = source_;
    roots.value = roots_;
    reconcileFocus(keepKey);
  }

  /** 顶栏用：按平台统计数量，顺序固定，没有的平台不出现。 */
  const providerCounts = computed(() => {
    const counts = new Map<string, number>();
    for (const g of allGames.value) {
      if (hidden.value.has(g.key)) continue;
      counts.set(g.provider, (counts.get(g.provider) ?? 0) + 1);
    }
    return (["steam", "epic", "wegame", "manual"] as const)
      .filter((id) => counts.has(id))
      .map((id) => ({ id, count: counts.get(id) ?? 0 }));
  });

  /**
   * orderGames 排序：收藏永远置顶，同类之内按用户选的字段与方向。
   *
   * 排序放在数据层而不是渲染层：卡片下标就是焦点下标，
   * 两边顺序不一致的话 ←→ 会跳着走。
   */
  function orderGames(list: Game[]): Game[] {
    const dir = sortDir.value === "asc" ? 1 : -1;
    const by = sortBy.value;

    // 自定义顺序：完全按玩家拖出来的位置，收藏不再置顶
    // （不然「把收藏挪下去」会一直被弹回顶部）。
    if (by === "manual" && order.value.length) {
      const pos = new Map(order.value.map((key, i) => [key, i]));
      const at = (g: Game) => pos.get(g.key) ?? Number.MAX_SAFE_INTEGER;
      return [...list].sort((a, b) => {
        const pa = at(a);
        const pb = at(b);
        if (pa !== pb) return pa - pb;
        // 没排过的（新装的、手加的）排在后面，彼此按名字稳定
        return a.title.localeCompare(b.title, "zh-Hans-CN");
      });
    }

    const value = (g: Game): string | number => {
      switch (by) {
        case "name":
          return g.title.toLowerCase();
        case "playtime":
          return g.playtimeMinutes ?? 0;
        case "size":
          return g.sizeBytes ?? 0;
        case "recent":
        default:
          return g.lastPlayed ? Date.parse(g.lastPlayed) : 0;
      }
    };

    return [...list].sort((a, b) => {
      const fa = favorites.value.has(a.key) ? 0 : 1;
      const fb = favorites.value.has(b.key) ? 0 : 1;
      if (fa !== fb) return fa - fb; // 收藏置顶，不受排序方向影响

      const va = value(a);
      const vb = value(b);
      if (typeof va === "string" || typeof vb === "string") {
        return String(va).localeCompare(String(vb), "zh-Hans-CN") * dir;
      }
      if (va === vb) {
        // 同分（比如都没玩过）时按名字兜底，保证顺序稳定
        return a.title.localeCompare(b.title, "zh-Hans-CN");
      }
      return (va - vb) * dir;
    });
  }

  /** reconcileFocus 让焦点跟着数据走：尽量停在 keepKey 上。 */
  function reconcileFocus(keepKey?: string) {
    const list = games.value;
    if (!list.length) {
      index.value = 0;
      return;
    }
    const found = keepKey ? list.findIndex((g) => g.key === keepKey) : -1;
    index.value = found >= 0 ? found : Math.min(index.value, list.length - 1);
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

    const game = allGames.value.find((g) => g.key === target);
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
    const game = allGames.value.find((g) => g.key === update.key);
    if (!game) return;
    game.artwork = update.artwork;
    if (update.accent) game.accent = update.accent;
  }

  /** setRunning 更新「是否正在运行」，卡片上会显示小圆点。 */
  function setRunning(key: string, running: boolean) {
    const game = allGames.value.find((g) => g.key === key);
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
    reconcileFocus(target);
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
    profileName,
    profileAvatar,
    profileInitial,
    providerCounts,
    hidden,
    hiddenCount,
    hiddenGames,
    sortBy,
    sortDir,
    order,
    setManualOrder,
    platformFilter,
    toggleHidden,
    unhideAll,
    cycleSortBy,
    toggleSortDir,
    cyclePlatformFilter,
    setPlatformFilter,
    cycleIconPos,
    setNickname,
    pickAvatar,
    clearAvatar,
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
 *
 * 和 Go 那边的 library.DemoGames 保持同一个意思：平台写真的（顶栏统计、
 * 平台图标、筛选都按真库的样子走），只是 `demo: true`，什么也启动不了。
 * 封面直接借 Steam CDN（同一部游戏的库图），调样式时不会看到一排渐变色块。
 */
function browserDemo(): Game[] {
  const cdn = "https://cdn.cloudflare.steamstatic.com/steam/apps/";
  const rows: Array<[string, string, string, number, number, number, string[]?]> = [
    // [artID, 标题, 平台, 几天前, 游玩分钟, 体积 GB]
    ["1245620", "艾尔登法环", "steam", 1, 4210, 60.2, ["魂系"]],
    ["1086940", "博德之门 3", "steam", 3, 6120, 122.4, ["RPG"]],
    ["1091500", "赛博朋克 2077", "steam", 6, 1680, 70.8],
    ["1145350", "哈迪斯 II", "steam", 14, 540, 8.6],
    ["1087100", "心灵杀手 2", "epic", 9, 960, 90, ["Epic 独占"]],
    ["1850570", "死亡搁浅 导演剪辑版", "epic", 30, 3300, 80.1],
    ["2358720", "黑神话：悟空", "wegame", 21, 2100, 130, ["国产"]],
    ["838350", "太吾绘卷", "wegame", 45, 1500, 6.2, ["国产"]],
  ];
  return rows.map(([artID, title, provider, days, minutes, sizeGB, tags], i) => ({
    key: `demo:${provider}:${artID}`,
    title,
    provider,
    appId: provider === "steam" ? artID : undefined,
    artwork: {
      cover: `${cdn}${artID}/library_600x900.jpg`,
      hero: `${cdn}${artID}/library_hero.jpg`,
    },
    accent: browserAccents[i],
    installed: true,
    running: false,
    demo: true,
    playtimeMinutes: minutes,
    sizeBytes: Math.round(sizeGB * 1024 ** 3),
    tags,
    lastPlayed: new Date(Date.now() - days * 86_400_000).toISOString(),
  }));
}

/** 和上面一一对应的主色：演示数据没有本地封面可抽色，直接写死。 */
const browserAccents = [
  "#c9a227",
  "#a5642f",
  "#e0c341",
  "#c0392b",
  "#6c7f9a",
  "#4f7f8a",
  "#b8863b",
  "#7a8f4f",
];
