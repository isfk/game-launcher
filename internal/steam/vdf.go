// Package steam 读 Steam 客户端留在本机的库元数据：
// VDF（libraryfolders.vdf）与 ACF（appmanifest_<appid>.acf）。
// 只读文件，不登录、不调 Web API —— 所以断网也能用。
package steam

import (
	"fmt"
)

// Object 是一棵 VDF 对象：键映射到 string 或嵌套的 Object。
type Object map[string]any

// Str 取一个字符串值，缺失时返回空。
func (o Object) Str(key string) string {
	s, _ := o[key].(string)
	return s
}

// Obj 取一个嵌套对象，缺失时返回 nil。
func (o Object) Obj(key string) Object {
	obj, _ := o[key].(Object)
	return obj
}

// Parse 解析一份 VDF 文本。支持：引号字符串、裸词、嵌套对象、
// `//` 行注释、以及 Windows 换行。
func Parse(data []byte) (Object, error) {
	p := &parser{data: data}
	if err := p.skipSpace(); err != nil {
		return nil, err
	}
	// 顶层可能被一个名字包着（libraryfolders.vdf 就是这样的）。
	obj, err := p.object(true)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

type parser struct {
	data []byte
	pos  int
}

// object 解析到匹配的 '}' 或输入结束。top 为真时缺少 '}' 不算错：
// Steam 的 VDF 文件整体被一个根键包着，解析结果是 {"根键": {...}}。
func (p *parser) object(top bool) (Object, error) {
	obj := Object{}
	for {
		if err := p.skipSpace(); err != nil {
			return nil, err
		}
		if p.pos >= len(p.data) {
			if top {
				return obj, nil
			}
			return nil, fmt.Errorf("steam: vdf 意外结束于 %d 字节处", p.pos)
		}
		if p.data[p.pos] == '}' {
			p.pos++
			if top {
				return nil, fmt.Errorf("steam: 顶层多余的 '}'")
			}
			return obj, nil
		}

		key, _, err := p.token()
		if err != nil {
			return nil, err
		}

		if err := p.skipSpace(); err != nil {
			return nil, err
		}
		if p.pos >= len(p.data) {
			return nil, fmt.Errorf("steam: 键 %q 之后没有值", key)
		}

		if p.data[p.pos] == '{' {
			p.pos++
			child, err := p.object(false)
			if err != nil {
				return nil, err
			}
			obj[key] = child
			continue
		}

		val, _, err := p.token()
		if err != nil {
			return nil, err
		}
		obj[key] = val
	}
}

// token 读一个字符串或裸词。
func (p *parser) token() (string, bool, error) {
	if p.pos >= len(p.data) {
		return "", false, fmt.Errorf("steam: 缺少 token")
	}
	if p.data[p.pos] != '"' {
		start := p.pos
		for p.pos < len(p.data) {
			c := p.data[p.pos]
			if c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '{' || c == '}' || c == '"' {
				break
			}
			p.pos++
		}
		if start == p.pos {
			return "", false, fmt.Errorf("steam: %d 字节处有无法识别的字符 %q", p.pos, p.data[p.pos])
		}
		return string(p.data[start:p.pos]), false, nil
	}

	p.pos++ // 开头的引号
	var out []byte
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		if c == '\\' && p.pos+1 < len(p.data) {
			p.pos++
			switch p.data[p.pos] {
			case 'n':
				out = append(out, '\n')
			case 't':
				out = append(out, '\t')
			default: // \" \\ 等
				out = append(out, p.data[p.pos])
			}
			p.pos++
			continue
		}
		if c == '"' {
			p.pos++
			return string(out), true, nil
		}
		out = append(out, c)
		p.pos++
	}
	return "", false, fmt.Errorf("steam: 引号字符串没有结束")
}

// skipSpace 跳过空白与 // 注释。
func (p *parser) skipSpace() error {
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			p.pos++
		case c == '/' && p.pos+1 < len(p.data) && p.data[p.pos+1] == '/':
			for p.pos < len(p.data) && p.data[p.pos] != '\n' {
				p.pos++
			}
		default:
			return nil
		}
	}
	return nil
}
