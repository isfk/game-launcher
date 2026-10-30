package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"

	"game-launcher/internal/artcache"
	"game-launcher/internal/asset"
	"game-launcher/internal/config"
	"game-launcher/internal/epic"
	"game-launcher/internal/launch"
	"game-launcher/internal/library"
	"game-launcher/internal/manual"
	"game-launcher/internal/pad"
	"game-launcher/internal/proc"
	"game-launcher/internal/steam"
	"game-launcher/internal/storeart"
	"game-launcher/internal/wegame"
)

// artStore 把本地的封面文件变成页面能加载的 URL（见 internal/asset）。
// 页面拿到的 artwork 字段都是 art://localhost/<id> 这样的地址。
var artStore = asset.NewStore()

// ArtworkUpdate 是一部游戏的美术资源换成「本地缓存」后的通知：
// 页面收到就把这张卡换成缓存版，同时用新的主色更新背景。
type ArtworkUpdate struct {
	Key     string          `json:"key"`
	Artwork library.Artwork `json:"artwork"`
	Accent  string          `json:"accent,omitempty"`
}

// ArtworkReady 美术资源缓存完成后广播给页面。
var ArtworkReady = mygo.NewEvent[ArtworkUpdate]("artworkReady")

// GameState 是「某部游戏开始 / 结束运行」的通知。
type GameState struct {
	Key     string `json:"key"`
	Title   string `json:"title"`
	Running bool   `json:"running"`
}

// GameRunning 游戏开始或退出时广播给页面。
var GameRunning = mygo.NewEvent[GameState]("gameRunning")

// LibraryChanged 在后台监听到 Steam 库变化（装了新游戏、卸载、换库位置）
// 并重新扫描完之后广播 —— 界面收到就地替换列表，不用重启应用。
var LibraryChanged = mygo.NewEvent[LibraryResult]("libraryChanged")

// LibraryResult 是 Launcher.Library 的返回。
type LibraryResult struct {
	Games []library.Game `json:"games"`
	// Source 是这批数据从哪来："steam" 是真扫到的，"demo" 是演示数据。
	Source string `json:"source"`
	// Roots 是这次扫过的 Steam 目录，扫不到东西时用它排查。
	Roots []string `json:"roots"`
	// ScannedAt 是扫描时刻。
	ScannedAt time.Time `json:"scannedAt"`
}

// Launcher 是页面能调用的全部后端能力。
// 方法在各自的 goroutine 上跑，所以慢活（扫盘、下载封面）可以直接阻塞。
type Launcher struct {
	mu        sync.Mutex
	games     []library.Game
	source    string
	scanned   time.Time
	signature string // 上次扫描时的库指纹，后台监听用它判断要不要重扫

	storeArt *storeart.Client // 商店图查询（新游戏还没上传库封面时的兜底）
	// covers 记住每部游戏封面在本机的真实路径（给取主色用），
	// 因为返回给页面的 artwork 已经换成 URL 了。
	covers map[string]string
	// accents 缓存从封面提取出的主色。
	accents map[string]string
	// cache 是封面 / hero 的本地缓存，App 起来之后才有。
	cache *artcache.Cache
	// win 是主窗口，游戏退出后要把它拉回前台。
	win *mygo.Window

	// cfg 是设置的落盘位置，settings 是内存里的当前值（第一次用到时加载）。
	cfg            *config.Store
	settings       config.Settings
	settingsLoaded bool

	// focused 是窗口是否在前台。手柄是全局轮询的：游戏开着的时候按一下 A，
	// 启动器也照样能读到。不在前台就当成没接手柄，界面不会误触。
	focused atomic.Bool
	// starting 是「已经发出启动请求、但进程还没被确认」的短暂窗口，
	// 用来挡住连点两下 A 导致的重复启动。
	starting bool
	// runningKey 是正在运行的游戏 key（空表示没有）。
	// 有游戏在跑的时候不接受新的启动请求，这是防双开的第二道闸。
	runningKey string
}

// newLauncher 建一个服务实例。main 里绑定的是它。
func newLauncher() *Launcher {
	return &Launcher{covers: make(map[string]string), accents: make(map[string]string)}
}

// setCache 由 main 在 App ready 之后调用：缓存目录要靠运行中的 App 才拿得到。
func (l *Launcher) setStoreArt(c *storeart.Client) {
	l.mu.Lock()
	l.storeArt = c
	l.mu.Unlock()
}

// setConfig 由 main 传入设置文件位置（配置目录要靠运行中的 App 才拿得到）。
func (l *Launcher) setConfig(store *config.Store) {
	l.mu.Lock()
	l.cfg = store
	l.settingsLoaded = false
	l.mu.Unlock()
}

// settingsNow 取当前设置，第一次调用时从磁盘加载（加载失败就用默认值）。
// 调用前必须持有 l.mu。
func (l *Launcher) settingsNow() config.Settings {
	if l.settingsLoaded {
		return l.settings
	}
	l.settings = config.Default()
	l.settingsLoaded = true
	if l.cfg == nil {
		return l.settings
	}
	if loaded, err := l.cfg.Load(); err != nil {
		// 设置读坏了不该让应用起不来：用默认值继续，日志里留个痕
		log.Printf("读取设置失败，本次用默认值: %v", err)
	} else {
		l.settings = loaded
	}
	return l.settings
}

// saveSettings 落盘并更新内存里的值。调用前必须持有 l.mu。
func (l *Launcher) saveSettings(next config.Settings) (config.Settings, error) {
	l.settings = next
	l.settingsLoaded = true
	if l.cfg == nil {
		return next, nil // 没有落盘位置（测试环境）：只保留在内存里
	}
	if err := l.cfg.Save(next); err != nil {
		return next, err
	}
	return next, nil
}

// Settings 返回当前设置。页面启动时拿它来恢复音效开关和收藏。
func (l *Launcher) Settings() config.Settings {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.settingsNow()
}

// SetSoundEnabled 记住界面音效开关。
func (l *Launcher) SetSoundEnabled(enabled bool) (config.Settings, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	next := l.settingsNow()
	next.SoundEnabled = enabled
	return l.saveSettings(next)
}

// SetFavorite 记住 / 取消一部游戏的收藏。
//
// 收藏只跟 key 有关，不需要游戏还在库里 —— 卸载了再装回来，
// 收藏还在（Steam 那边也是这个行为）。
func (l *Launcher) SetFavorite(key string, favorite bool) (config.Settings, error) {
	if key == "" {
		return l.Settings(), fmt.Errorf("收藏需要游戏 key")
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	next := l.settingsNow()
	// 复制一份再改：settingsNow 返回的结构体里切片是和 l.settings 共享的
	keys := make([]string, 0, len(next.Favorites)+1)
	for _, k := range next.Favorites {
		if k != key {
			keys = append(keys, k)
		}
	}
	if favorite {
		keys = append(keys, key)
	}
	next.Favorites = keys
	return l.saveSettings(next)
}

func (l *Launcher) setCache(c *artcache.Cache) {
	l.mu.Lock()
	l.cache = c
	l.mu.Unlock()
}

// attach 记住主窗口，供「游戏退出回前台」使用。
func (l *Launcher) attach(win *mygo.Window) {
	l.mu.Lock()
	l.win = win
	l.mu.Unlock()
	// 窗口刚起来时它就在前台；先当成有焦点，第一次轮询会纠正过来
	l.focused.Store(true)
}

// SetFocused 由窗口的焦点事件调用。
func (l *Launcher) SetFocused(on bool) { l.focused.Store(on) }

// focusedNow 报告窗口此刻是否在前台。
//
// 优先问窗口本身（Windows 上是 GetForegroundWindow，最实在），
// 问不到就退回事件记下的值；连窗口都还没有（比如测试里）就当有焦点，
// 免得把手柄功能整个关掉。
func (l *Launcher) focusedNow() bool {
	l.mu.Lock()
	win := l.win
	l.mu.Unlock()
	if win != nil {
		focused := win.IsFocused()
		l.focused.Store(focused)
		return focused
	}
	return l.focused.Load()
}

// Library 返回启动器的游戏库。refresh 为 true 时重新扫磁盘，
// 否则用上一次的结果（首次调用总会扫描）。
func (l *Launcher) Library(ctx context.Context, refresh bool) (LibraryResult, error) {
	l.mu.Lock()
	if l.games != nil && !refresh {
		result := l.result()
		l.mu.Unlock()
		return result, nil
	}
	cache := l.cache
	l.mu.Unlock()

	return l.rescan(ctx, cache)
}

// rescan 扫一遍磁盘、更新缓存，并把结果整理成界面能用的形状。
func (l *Launcher) rescan(ctx context.Context, cache *artcache.Cache) (LibraryResult, error) {
	// 各平台的扫描互不干扰：这台机器没装 Epic 不该影响 Steam 的库。
	var games []library.Game
	sources := make([]string, 0, 2)
	for _, p := range providers() {
		found, err := p.Scan(ctx)
		if err != nil {
			// 扫不到不是错误：空库是合法状态，Roots 留给界面排查
			log.Printf("library: %s 扫描失败（已跳过）: %v", p.ID(), err)
		}
		if len(found) == 0 {
			continue
		}
		sources = append(sources, p.ID())
		games = append(games, found...)
	}
	source := strings.Join(sources, "+")

	// 手动添加的游戏排在前面：它们是玩家特意手动加进来的，
	// 不该被 Steam 那几十部淹掉。
	manualGames := l.manualGames()
	if len(manualGames) > 0 {
		games = append(manualGames, games...)
		if source == "" {
			source = "manual" // 只有手动游戏，不该显示成演示数据
		}
	}

	if len(games) == 0 {
		// 这台机器上什么库都没有：给界面一份演示数据，好看清版式。
		games = library.DemoGames()
		source = "demo"
	}

	// 新发售的游戏常常还没上传库封面（library_600x900.jpg 直接 404），
	// 这时候去商店查一次，用商店图顶上。
	l.fillStoreArtwork(ctx, cache, games)

	// 已经缓存到本地的图先用上：第二次启动就能完全离线、秒开。
	resolveCachedArtwork(cache, games)

	l.mu.Lock()
	l.publish(games)
	l.source = source
	l.scanned = time.Now()
	l.signature = librarySignature()
	result := l.result()
	l.mu.Unlock()

	// 剩下的（本机没有、走 CDN 的）在后台慢慢补齐，补好一张通知一张。
	go l.warmArtwork(games)

	return result, nil
}

// manualGames 从设置里取出手动添加的游戏（转换成界面认的 Game）。
func (l *Launcher) manualGames() []library.Game {
	l.mu.Lock()
	settings := l.settingsNow()
	l.mu.Unlock()
	return manual.Games(settings.ManualGames)
}

// AddManualGame 弹系统文件选择框，把选中的可执行文件加成一部游戏。
//
// 取消选择不算错误：返回 nil error + 原样的游戏库，界面什么都不用做。
func (l *Launcher) AddManualGame(ctx context.Context) (LibraryResult, error) {
	l.mu.Lock()
	parent := l.win
	cache := l.cache
	l.mu.Unlock()

	picked, err := pickExecutable(parent)
	if err != nil {
		return LibraryResult{}, err
	}
	if picked == "" {
		// 用户按了取消
		result, _ := l.Library(ctx, false)
		return result, nil
	}
	if err := manual.ValidPath(picked); err != nil {
		return LibraryResult{}, err
	}

	entry := manual.Entry(picked, "")
	l.mu.Lock()
	settings := l.settingsNow()
	// 同一个文件重复添加就直接返回，不产生第二条
	exists := false
	for _, g := range settings.ManualGames {
		if g.ID == entry.ID {
			exists = true
			break
		}
	}
	if !exists {
		settings.ManualGames = append(settings.ManualGames, entry)
		if _, saveErr := l.saveSettings(settings); saveErr != nil {
			l.mu.Unlock()
			return LibraryResult{}, fmt.Errorf("保存失败: %w", saveErr)
		}
	}
	l.mu.Unlock()

	if exists {
		return l.Library(ctx, false)
	}
	// 重新扫一遍：新加的那部会作为第一张卡出现
	return l.rescan(ctx, cache)
}

// RemoveManualGame 把一部手动添加的游戏从库里移除（不动磁盘上的文件）。
func (l *Launcher) RemoveManualGame(ctx context.Context, key string) (LibraryResult, error) {
	l.mu.Lock()
	settings := l.settingsNow()
	kept := make([]config.ManualGame, 0, len(settings.ManualGames))
	found := false
	for _, g := range settings.ManualGames {
		if manual.IDFor(g.Path) == key {
			found = true
			continue
		}
		kept = append(kept, g)
	}
	if !found {
		l.mu.Unlock()
		return LibraryResult{}, fmt.Errorf("库里没有这条手动添加的记录: %s", key)
	}
	settings.ManualGames = kept
	// 顺手把收藏里的这条也清掉，免得收藏列表里留一个点不开的 key
	favorites := make([]string, 0, len(settings.Favorites))
	for _, f := range settings.Favorites {
		if f != key {
			favorites = append(favorites, f)
		}
	}
	settings.Favorites = favorites
	if _, err := l.saveSettings(settings); err != nil {
		l.mu.Unlock()
		return LibraryResult{}, fmt.Errorf("保存失败: %w", err)
	}
	cache := l.cache
	l.mu.Unlock()

	return l.rescan(ctx, cache)
}

// pickExecutable 弹系统文件选择框，返回选中的路径（取消时返回空串）。
//
// 抽成变量是为了测试能替换掉它：真实的对话框要人点，没法在测试里跑。
var pickExecutable = func(parent *mygo.Window) (string, error) {
	opts := mygo.OpenDialogOptions{
		Parent:  parent,
		Title:   "选择要添加的游戏",
		Message: "支持 .app（macOS）和 .exe / .bat / .cmd / .lnk（Windows）",
		Filters: manualFilters(),
	}
	paths, err := mygo.Dialog.Open(opts)
	if err != nil {
		return "", fmt.Errorf("打开文件选择框失败: %w", err)
	}
	if len(paths) == 0 {
		return "", nil
	}
	return paths[0], nil
}

// manualFilters 按平台给文件选择框配扩展名过滤。
// macOS 不配过滤：Unix 程序常常没有后缀，配了反而选不到。
func manualFilters() []mygo.FileFilter {
	if runtime.GOOS == "windows" {
		return []mygo.FileFilter{{Name: "可执行文件", Extensions: []string{"exe", "bat", "cmd", "lnk"}}}
	}
	return nil
}

// providers 返回要扫的平台。加新平台只要在这里追加一行，界面一行不用改。
func providers() []library.Provider {
	return []library.Provider{
		steam.Provider{},
		epic.Provider{},
		wegame.Provider{},
	}
}

// librarySignature 把各平台的库指纹拼起来，供后台轮询判断要不要重扫。
func librarySignature() string {
	return steam.Signature() + "|epic|" + epic.SignatureNow() + "|wegame|" + wegame.SignatureNow()
}

// WatchLibrary 每 5 秒给 Steam 库拍一次指纹，一变就重扫并广播 LibraryChanged。
// 这样在 Steam 里装完一部游戏，回到启动器不用按任何键它自己就出现了。
//
// 用指纹而不是文件监听，是因为：跨平台零依赖、Windows 上也不用处理
// ReadDirectoryChangesW 的缓冲区溢出；一轮只是几十上百次 stat，开销可忽略。
func (l *Launcher) WatchLibrary(ctx context.Context) {
	const interval = 5 * time.Second

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		l.mu.Lock()
		seen := l.signature
		cache := l.cache
		l.mu.Unlock()

		current := librarySignature()
		if current == seen {
			continue
		}

		// Steam 装游戏时会连着写好几个文件，等它安静下来再扫，
		// 免得扫到「下了一半」的中间状态。
		time.Sleep(1500 * time.Millisecond)
		l.mu.Lock()
		l.signature = librarySignature()
		l.mu.Unlock()

		result, err := l.rescan(ctx, cache)
		if err != nil {
			log.Printf("library: 后台重扫失败: %v", err)
			continue
		}
		if err := LibraryChanged.Broadcast(result); err != nil {
			log.Printf("library: 广播失败: %v", err)
		}
	}
}

// publish 记下这批游戏，并把封面路径换成页面能加载的 URL。
// 调用方必须持有 l.mu。
func (l *Launcher) publish(games []library.Game) {
	l.covers = make(map[string]string, len(games))
	for i := range games {
		game := &games[i]
		if cover := game.Artwork.Cover; cover != "" && !isURL(cover) {
			l.covers[game.Key] = cover
		}
		game.Artwork = library.Artwork{
			Cover: serveLocalOrKeep(game.Artwork.Cover),
			Hero:  serveLocalOrKeep(game.Artwork.Hero),
			Logo:  serveLocalOrKeep(game.Artwork.Logo),
			Icon:  serveLocalOrKeep(game.Artwork.Icon),
		}
	}
	l.games = games
}

// fillStoreArtwork 给「本地还没有封面」的游戏补一份 Store 封面。
//
// 为什么要在这里做，而不是等后台下载失败再补：
// 后台补图靠 ArtworkReady 事件通知页面，而首次启动时页面还没订阅，
// 事件会丢 —— 卡面就一直空着，得重启一次才正常。
//
// 查询结果和图片本身都有磁盘缓存，所以第二次启动是纯粹的本地读，不联网。
// 设 LAUNCHER_NO_STORE_ASSETS=1 可以完全关掉联网。
func (l *Launcher) fillStoreArtwork(ctx context.Context, cache *artcache.Cache, games []library.Game) {
	l.mu.Lock()
	client := l.storeArt
	l.mu.Unlock()

	if client == nil || os.Getenv("LAUNCHER_NO_STORE_ASSETS") == "1" {
		return
	}

	// 一次启动最多等 8 秒：宁可这次先显示渐变占位，
	// 也不能让首页卡在「正在加载」上。
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup

	for i := range games {
		game := &games[i]
		// 封面和 hero 都已经在本地就不用查了 —— 这条路完全不联网。
		if game.AppID == "" || (hasLocalArt(cache, *game, game.Artwork.Cover) &&
			hasLocalArt(cache, *game, game.Artwork.Hero)) {
			continue
		}
		wg.Add(1)
		go func(target *library.Game) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			assets, ok := client.Lookup(ctx, target.AppID)
			if !ok {
				return
			}
			// 走到这里的都是本地没封面的：CDN 上那个地址多半是 404，
			// 直接换成 Store 给的地址（之后由 resolveCachedArtwork 换成本地路径）。
			if assets.Capsule != "" {
				target.Artwork.Cover = assets.Capsule
			}
			// hero 同理：扫描时拼出的 library_hero.jpg 对新产品是 404，
			// 但本地若已经有下载好的 hero 就别覆盖。
			if assets.Hero != "" && !hasLocalArt(cache, *target, target.Artwork.Hero) {
				target.Artwork.Hero = assets.Hero
			}
		}(game)
	}
	wg.Wait()
}

// hasLocalArt 看这张图是不是已经在本地缓存里。
func hasLocalArt(cache *artcache.Cache, game library.Game, url string) bool {
	if cache == nil || game.AppID == "" || url == "" {
		return false
	}
	_, ok := cache.Lookup(game.AppID, path.Base(url))
	return ok
}

// resolveCachedArtwork 把 CDN 地址里「已经下载过」的那些换成本地路径。
func resolveCachedArtwork(cache *artcache.Cache, games []library.Game) {
	if cache == nil {
		return
	}
	for i := range games {
		game := &games[i]
		if game.AppID == "" {
			continue
		}
		for _, field := range []*string{&game.Artwork.Cover, &game.Artwork.Hero, &game.Artwork.Logo} {
			if !strings.HasPrefix(*field, "https://") {
				continue
			}
			if local, ok := cache.Lookup(game.AppID, path.Base(*field)); ok {
				*field = local
			}
		}
	}
}

// warmArtwork 把走 CDN 的美术资源下载到本地缓存，逐个通知页面。
func (l *Launcher) warmArtwork(games []library.Game) {
	l.mu.Lock()
	cache := l.cache
	l.mu.Unlock()
	if cache == nil {
		return
	}

	sem := make(chan struct{}, 3) // 别把带宽和 CDN 打满
	var wg sync.WaitGroup
	for _, game := range games {
		if game.Provider != "steam" || game.AppID == "" {
			continue
		}
		wg.Add(1)
		go func(g library.Game) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			l.warmGame(cache, g)
		}(game)
	}
	wg.Wait()
}

// warmGame 处理一部游戏：下载还缺的图，有变化就通知页面。
func (l *Launcher) warmGame(cache *artcache.Cache, game library.Game) {
	updated := game.Artwork
	changed := false
	accent := ""

	// 封面：先试 CDN 上的库封面（library_600x900.jpg）。
	// 刚发售的游戏往往还没上传，会 404 —— 这时去商店查一次，用商店图顶上。
	if local, ok := l.fetchSlot(cache, game.AppID, game.Artwork.Cover); ok {
		updated.Cover = artStore.URL(local)
		changed = true
		accent = l.rememberCover(game.Key, local)
	} else if assets, ok := l.storeArtwork(game); ok {
		if local, ok := l.fetchSlot(cache, game.AppID, assets.Capsule); ok {
			updated.Cover = artStore.URL(local)
			changed = true
			accent = l.rememberCover(game.Key, local)
		}
		if local, ok := l.fetchSlot(cache, game.AppID, assets.Hero); ok {
			updated.Hero = artStore.URL(local)
			changed = true
		}
	}

	if updated.Hero == "" {
		if local, ok := l.fetchSlot(cache, game.AppID, game.Artwork.Hero); ok {
			updated.Hero = artStore.URL(local)
			changed = true
		}
	}
	if local, ok := l.fetchSlot(cache, game.AppID, game.Artwork.Logo); ok {
		updated.Logo = artStore.URL(local)
		changed = true
	}

	if !changed {
		return
	}

	l.mu.Lock()
	for i := range l.games {
		if l.games[i].Key == game.Key {
			l.games[i].Artwork = updated
		}
	}
	l.mu.Unlock()

	_ = ArtworkReady.Broadcast(ArtworkUpdate{Key: game.Key, Artwork: updated, Accent: accent})
}

// fetchSlot 下载一张图，成功时返回本地路径。
// 非 CDN 地址（已经指向本地缓存）和空地址都当作「没有」。
func (l *Launcher) fetchSlot(cache *artcache.Cache, appID, url string) (string, bool) {
	if cache == nil || appID == "" || !strings.HasPrefix(url, "https://") {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	local, err := cache.Fetch(ctx, appID, path.Base(url), url)
	if err != nil {
		return "", false
	}
	return local, true
}

// rememberCover 记下封面路径并提取主色，返回主色（拿不到就是空串）。
func (l *Launcher) rememberCover(key, local string) string {
	l.mu.Lock()
	l.covers[key] = local
	l.mu.Unlock()

	color, err := steam.AccentFromImage(local)
	if err != nil {
		return ""
	}
	l.mu.Lock()
	l.accents[key] = color
	l.mu.Unlock()
	return color
}

// storeArtwork 查商店图（结果有磁盘缓存，正常情况下不联网）。
func (l *Launcher) storeArtwork(game library.Game) (storeart.Assets, bool) {
	l.mu.Lock()
	client := l.storeArt
	l.mu.Unlock()

	if client == nil || game.AppID == "" || os.Getenv("LAUNCHER_NO_STORE_ASSETS") == "1" {
		return storeart.Assets{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return client.Lookup(ctx, game.AppID)
}

// serveLocalOrKeep 把本地路径换成 art:// URL；本来就是 URL 的（CDN）原样透传。
func serveLocalOrKeep(value string) string {
	if value == "" {
		return ""
	}
	if isURL(value) {
		return value
	}
	return artStore.URL(value)
}

func isURL(value string) bool {
	return strings.HasPrefix(value, "http://") ||
		strings.HasPrefix(value, "https://") ||
		strings.HasPrefix(value, asset.Scheme+"://")
}

// Accent 返回一部游戏的主色，从它的本地封面提取，结果缓存。
// 没有封面或者解不出颜色时返回空串，界面会退回按标题散列的颜色。
func (l *Launcher) Accent(ctx context.Context, key string) (string, error) {
	l.mu.Lock()
	cached, ok := l.accents[key]
	cover := l.covers[key]
	l.mu.Unlock()

	if ok {
		return cached, nil
	}
	if cover == "" {
		return "", nil
	}

	color, err := steam.AccentFromImage(cover)
	if err != nil {
		return "", nil // 取不到颜色不算错误，界面有兜底
	}

	l.mu.Lock()
	l.accents[key] = color
	l.mu.Unlock()
	return color, nil
}

// result 拼返回，调用方必须持有 l.mu。
func (l *Launcher) result() LibraryResult {
	roots := steam.DefaultRoots()
	if env := steam.RootFromEnv(); env != "" {
		roots = append([]string{env}, roots...)
	}
	roots = append(roots, epic.DefaultRoots()...)
	roots = append(roots, wegame.DefaultRoots()...)
	if env := epic.RootFromEnv(); env != "" {
		roots = append([]string{env}, roots...)
	}
	if env := wegame.RootFromEnv(); env != "" {
		roots = append([]string{env}, roots...)
	}
	return LibraryResult{
		Games:     l.games,
		Source:    l.source,
		Roots:     roots,
		ScannedAt: l.scanned,
	}
}

// Launch 启动一部游戏，返回它的名字（界面用来显示「正在启动 X」）。
func (l *Launcher) Launch(ctx context.Context, key string) (string, error) {
	game, found := l.gameByKey(key)
	if !found {
		return "", fmt.Errorf("找不到游戏 %q，先刷新游戏库", key)
	}

	// 防双开：手柄的 A 很容易被连按，或者启动器还没退到后台时又按了一下。
	// 有一部在跑就不再开第二份；刚发出启动请求的那几秒也挡住。
	l.mu.Lock()
	if l.runningKey != "" {
		running := l.titleOfLocked(l.runningKey)
		l.mu.Unlock()
		return "", fmt.Errorf("「%s」正在运行，先退出它再启动别的", running)
	}
	if l.starting {
		l.mu.Unlock()
		return "", fmt.Errorf("「%s」正在启动，稍等一下", game.Title)
	}
	l.starting = true
	l.mu.Unlock()

	title, err := l.launchGame(ctx, game)
	if err != nil {
		l.mu.Lock()
		l.starting = false
		l.mu.Unlock()
		return "", err
	}
	return title, nil
}

// titleOfLocked 按 key 找标题（调用方已经持有锁）。
func (l *Launcher) titleOfLocked(key string) string {
	for _, g := range l.games {
		if g.Key == key {
			return g.Title
		}
	}
	return key
}

// launchGame 是 Launch 的实际派发，拆出来让上层能统一收尾 starting 标记。
func (l *Launcher) launchGame(ctx context.Context, game library.Game) (string, error) {
	switch game.Provider {
	case "steam":
		if err := (steam.Provider{}).Launch(ctx, game); err != nil {
			return "", err
		}
		// 开始守护这个进程：它退出时把启动器拉回前台。
		l.watchAfterLaunch(game)
		return game.Title, nil
	case "epic":
		// 走 openURL 这层间接（而不是 epic.Provider.Launch 里的 launch.OpenURL），
		// 测试才能替换掉「真的去开协议」这一步
		if err := openURL(ctx, epic.LaunchURL(game.AppID)); err != nil {
			return "", err
		}
		l.watchAfterLaunch(game)
		return game.Title, nil
	case "wegame":
		// WeGame 没有稳定的启动协议，直接跑扫描时找到的主程序
		if game.Exec == "" {
			return "", fmt.Errorf("「%s」没找到可执行文件，先在 WeGame 里启动一次", game.Title)
		}
		if err := openURL(ctx, game.Exec); err != nil {
			return "", err
		}
		l.watchAfterLaunch(game)
		return game.Title, nil
	case "manual":
		// 手动游戏直接交给系统打开：.app 走 LaunchServices，exe 走 ShellExecuteW
		path := l.manualPath(game.Key)
		if path == "" {
			return "", fmt.Errorf("「%s」的路径失效了，重新添加一次", game.Title)
		}
		if err := openURL(ctx, path); err != nil {
			return "", err
		}
		l.watchAfterLaunch(game)
		return game.Title, nil
	case "demo":
		return "", fmt.Errorf("「%s」是演示数据，扫到真正的 Steam 库后才能启动", game.Title)
	default:
		return "", fmt.Errorf("暂时不支持启动 %s 的游戏", game.Provider)
	}
}

// watchAfterLaunch 决定要不要起守护协程。
// 测试里关掉它：那段逻辑要等真实进程，会把测试拖住。
func (l *Launcher) watchAfterLaunch(game library.Game) {
	if !watchProcesses {
		// 不守护的时候，「正在启动」这个状态没人清，就地清掉
		l.mu.Lock()
		l.starting = false
		l.mu.Unlock()
		return
	}
	go l.watchGame(game)
}

// gameByKey 从当前游戏库里按 key 取一部游戏。
func (l *Launcher) gameByKey(key string) (library.Game, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, g := range l.games {
		if g.Key == key {
			return g, true
		}
	}
	return library.Game{}, false
}

// manualPath 按 key 找到手动游戏的真实路径。
func (l *Launcher) manualPath(key string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, g := range l.settingsNow().ManualGames {
		if manual.IDFor(g.Path) == key {
			return g.Path
		}
	}
	return ""
}

// openURL 是 launch.OpenURL 的间接层：测试里换成假的，免得真去开 Steam。
var openURL = launch.OpenURL

// Uninstall 让游戏平台自己卸载一部游戏，返回它的名字。
//
// 为什么不由启动器直接删目录：Steam 的卸载还要清 appmanifest、更新已安装
// 索引、处理云存档标记，自己 rm -rf 只会把客户端搞成「文件缺失」的半损状态。
// 所以这里只把请求交给客户端（它会再弹自己的确认框），界面那边另外加一次确认
// 是为了防止手柄误触。
//
// 卸载完成后不需要我们做什么：WatchLibrary 的指纹轮询会发现 appmanifest 少了，
// 广播 LibraryChanged，界面自动刷新，这张卡就消失了。
func (l *Launcher) Uninstall(ctx context.Context, key string) (string, error) {
	game, found := l.gameByKey(key)
	if !found {
		return "", fmt.Errorf("找不到游戏 %q，先刷新游戏库", key)
	}

	switch game.Provider {
	case "steam":
		if game.AppID == "" {
			return "", fmt.Errorf("「%s」缺少 appid，没法交给 Steam 卸载", game.Title)
		}
		if err := openURL(ctx, "steam://uninstall/"+game.AppID); err != nil {
			return "", err
		}
		return game.Title, nil
	case "epic":
		// 同样交给客户端：它要清自己的清单、注册表和云存档映射
		if err := openURL(ctx, epic.UninstallURL(game.AppID)); err != nil {
			return "", err
		}
		return game.Title, nil
	case "wegame":
		return "", fmt.Errorf("「%s」由 WeGame 管理，请在 WeGame 客户端里卸载", game.Title)
	case "manual":
		return "", fmt.Errorf("「%s」是手动添加的，只能用「移除」（不会删除磁盘上的文件）", game.Title)
	case "demo":
		return "", fmt.Errorf("「%s」是演示数据，没有可卸载的东西", game.Title)
	default:
		return "", fmt.Errorf("暂时不支持卸载 %s 的游戏", game.Provider)
	}
}

// watchProcesses 控制「等游戏退出再回前台」的后台协程要不要开。
//
// 测试里必须关掉：那段逻辑要等一个真实进程（最长 120 秒），
// 留着它测试跑完了也退不出去，看起来就像卡死。
var watchProcesses = true

// watchGame 等游戏进程出现、再等它消失，然后回到前台。
// steam:// 不给我们 PID，所以按「安装目录里还有没有活进程」来判断。
func (l *Launcher) watchGame(game library.Game) {
	if game.InstallDir == "" {
		return
	}

	// Steam 客户端冷启动可能要几十秒，给足时间
	if !waitUntil(120*time.Second, time.Second, func() bool { return proc.RunningIn(game.InstallDir) }) {
		// 一直没等到：可能启动失败，或者游戏在别的目录跑。
		// 不管哪种，「正在启动」都得放开，否则再也启动不了第二部。
		l.mu.Lock()
		l.starting = false
		l.mu.Unlock()
		return
	}

	l.mu.Lock()
	l.starting = false
	l.runningKey = game.Key
	l.mu.Unlock()

	_ = GameRunning.Broadcast(GameState{Key: game.Key, Title: game.Title, Running: true})

	for {
		time.Sleep(2 * time.Second)
		if !proc.RunningIn(game.InstallDir) {
			break
		}
	}

	l.mu.Lock()
	if l.runningKey == game.Key {
		l.runningKey = ""
	}
	l.mu.Unlock()

	_ = GameRunning.Broadcast(GameState{Key: game.Key, Title: game.Title, Running: false})
	l.bringToFront()
}

// bringToFront 把启动器拉回前台。
func (l *Launcher) bringToFront() {
	l.mu.Lock()
	win := l.win
	l.mu.Unlock()

	if win == nil || win.IsDestroyed() {
		return
	}
	win.Show()
	win.Focus()
}

// waitUntil 每隔 interval 检查一次，直到 check 为真或超时。
func waitUntil(timeout, interval time.Duration, check func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return true
		}
		time.Sleep(interval)
	}
	return false
}

// ToggleFullScreen 切换全屏，返回切换后的状态。
func (l *Launcher) ToggleFullScreen(ctx context.Context) (bool, error) {
	win := mygo.CallerWindow(ctx)
	if win == nil {
		return false, fmt.Errorf("没有窗口上下文")
	}
	win.ToggleFullScreen()
	return win.IsFullScreen(), nil
}

// IsFullScreen 报告当前是不是全屏。
func (l *Launcher) IsFullScreen(ctx context.Context) bool {
	win := mygo.CallerWindow(ctx)
	return win != nil && win.IsFullScreen()
}

// Pads 把手柄状态流给页面：60Hz 轮询，只有变化时才发一帧。
// 页面用 `for await` 迭代，关掉 channel 就结束。
func (l *Launcher) Pads(ctx context.Context, frames *mygo.Channel[[]pad.State]) error {
	ticker := time.NewTicker(time.Second / 60)
	defer ticker.Stop()

	last := "\x00" // 保证首页一定会发（空手柄也要让界面知道「没接手柄」）
	ticks := 0
	for {
		states := l.padStates()

		// 每 12 帧（约 200ms）跟窗口核一次真实焦点。
		// 焦点事件在某些平台不一定送得及时，轮询兜住它。
		if ticks%12 == 0 {
			l.focusedNow()
		}
		ticks++

		if sig := pad.Signature(states); sig != last {
			last = sig
			if err := frames.Send(states); err != nil {
				return err // 页面走了
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// padStates 返回此刻该推给页面的手柄状态。
//
// 窗口不在前台（也就是说玩家在游戏里）时返回空：手柄是全局轮询的，
// 不在这里挡住的话，游戏里按 A 会被启动器当成「再启动一次」。
// 返回空切片还会把界面上的「按住」状态清掉，回到前台时不会残留。
func (l *Launcher) padStates() []pad.State {
	return gatePadStates(l.focusedNow(), pad.Snapshot)
}

// gatePadStates 是 padStates 的纯逻辑部分，单独拆出来是为了能测。
func gatePadStates(focused bool, snapshot func() []pad.State) []pad.State {
	if !focused {
		return []pad.State{}
	}
	return snapshot()
}

// PadNames 列出当前接上的手柄，设置页用。
func (l *Launcher) PadNames() []string {
	states := pad.Snapshot()
	names := make([]string, 0, len(states))
	for _, s := range states {
		names = append(names, s.Name)
	}
	return names
}

// Quit 退出启动器。
func (l *Launcher) Quit() {
	mygo.App.Quit()
}
