package steam

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Signature 给本机的 Steam 库拍一个「指纹」：库目录、每个 appmanifest 的名字、
// 大小和修改时间。装游戏 / 删游戏 / 换库位置都会让它变。
//
// 启动器每几秒比一次指纹，变了才重新扫描 —— 比对指纹比扫一遍便宜得多，
// 这样新装的游戏不用重启应用就能出现在首页。
func Signature(roots ...string) string {
	if len(roots) == 0 {
		roots = DefaultRoots()
		if env := RootFromEnv(); env != "" {
			roots = append([]string{env}, roots...)
		}
	}

	var b strings.Builder
	for _, root := range dedupe(roots) {
		for _, lib := range libraries(root) {
			b.WriteString(lib)
			b.WriteByte('|')
			// libraryfolders.vdf 本身的变化（新加了一块盘）也算
			if st, err := os.Stat(filepath.Join(lib, "steamapps", "libraryfolders.vdf")); err == nil {
				b.WriteString(strconv.FormatInt(st.ModTime().UnixNano(), 10))
			}
			b.WriteByte('|')

			acfs, err := filepath.Glob(filepath.Join(lib, "steamapps", "appmanifest_*.acf"))
			if err != nil {
				continue
			}
			for _, acf := range acfs {
				st, err := os.Stat(acf)
				if err != nil {
					continue
				}
				b.WriteString(filepath.Base(acf))
				b.WriteByte(':')
				b.WriteString(strconv.FormatInt(st.Size(), 10))
				b.WriteByte(':')
				b.WriteString(strconv.FormatInt(st.ModTime().UnixNano(), 10))
				b.WriteByte(',')
			}
			b.WriteByte(';')
		}
	}
	return b.String()
}
