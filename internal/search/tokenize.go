// Package search 提供中文词法预处理与 FTS 查询构造。
//
// 为什么单独成包：索引侧（adapters/sqlite 写 memory_fts）与查询侧
// （context 构造 MATCH）必须使用**完全相同**的词元切分，否则两侧词元空间
// 不一致，检索会静默漏召回。放在任何一侧都会让另一侧反向依赖。
// 本包是叶子包（只依赖标准库），与 internal/pack 同级。
//
// 技术契约 §8.1：中文没有空格分词，采用纯 Go 预处理生成相邻双字词元，
// 英文保留规范化词元。不使用外部分词器——SQLite 驱动加载自定义分词器需要
// CGO，与无 CGO 构建的目标冲突；而 FTS5 自带的 unicode61 会把一整串汉字
// 当成**一个**词元，中文检索等于失效。因此由本包先切分，再交给 unicode61
// 索引，两边都只看到空格分隔的二元词。
package search

import (
	"strings"
	"unicode"
)

// MaxQueryTerms 是单次 MATCH 表达式的词元上限。
//
// 两个理由把它压到较小值，而不是"越多越好"：
//
//   - 代价：检索文本（最近若干轮正文）可能有上千字。全部展开成 OR 项后
//     FTS5 要为每个词元做一次词表查找并把命中集合合并排序，实测 96 词元比
//     32 词元贵约 10ms（万级语料），而召回结果没有变好。
//   - 精度：OR 项越多，命中集合越大，BM25 要在更多噪声里分辨相关度。
//
// 截断保留**前面**的词元而不是随机丢弃：调用方按相关度优先级排列
// （当前输入在最前，见 context.memoryQueryText），因此保留下来的恰好是
// 最该参与检索的那部分，同时同一输入始终得到同一结果（可复现）。
const MaxQueryTerms = 32

// Tokenize 把文本切成检索词元。
//
// 规则：
//   - 连续的表意文字（汉字/假名/谚文）切成**相邻双字**：铜钥匙 → 铜钥、钥匙。
//     单个汉字不成词元——单字会命中几乎所有文本，噪声大于收益（契约 §8.1
//     要求单字查询走实体或词法补充路径，而不是把单字塞进索引）。
//   - 连续的拉丁字母/数字取整串并小写化：GeoTIFF-2 → geotiff、2。
//   - 其余字符（标点、空白）只起分隔作用。
//
// 返回值保留重复词元：索引侧要保留词频（BM25 依赖 tf），查询侧由
// MatchExpr 去重。
func Tokenize(text string) []string {
	var out []string
	var han []rune
	var latin []rune

	flushHan := func() {
		for i := 0; i+1 < len(han); i++ {
			out = append(out, string(han[i:i+2]))
		}
		han = han[:0]
	}
	flushLatin := func() {
		if len(latin) > 0 {
			out = append(out, strings.ToLower(string(latin)))
			latin = latin[:0]
		}
	}

	for _, r := range text {
		switch {
		case isIdeographic(r):
			flushLatin()
			han = append(han, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushHan()
			latin = append(latin, r)
		default:
			flushHan()
			flushLatin()
		}
	}
	flushHan()
	flushLatin()
	return out
}

// isIdeographic 判断字符是否属于"需要双字切分"的表意文字。
func isIdeographic(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r)
}

// Tokens 返回去重后的词元（保持首次出现顺序），用于构造查询。
func Tokens(text string) []string {
	if text == "" {
		return nil
	}
	seen := make(map[string]bool, 64)
	out := make([]string, 0, 64)
	for _, t := range Tokenize(text) {
		if seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
		if len(out) >= MaxQueryTerms {
			break
		}
	}
	return out
}

// MatchExpr 把查询文本构造成 FTS5 的 MATCH 表达式。
//
// 每个词元都用双引号包成字符串字面量：FTS5 的 `AND`/`OR`/`NOT`/`NEAR`/`*`
// 等操作符因此不会被正文里的同形字误当语法（安全转义）。词元之间是 OR——
// 长查询的双字词元很多，全部 AND 几乎必然零命中；选择性交给 BM25 排序。
//
// 没有词元时返回空串（调用方据此走实体/短查询兜底，而不是执行一次
// 恒真空查询）。
func MatchExpr(query string) string {
	return MatchExprOf(Tokens(query))
}

// MatchExprOf 用给定的词元序列构造 MATCH 表达式。
//
// 顺序即优先级：调用方会把更重要的词元排在前面（例如按当前输入识别的实体名），
// 超出 MaxQueryTerms 的部分被截掉，因此先到的先保留。
func MatchExprOf(terms []string) string {
	if len(terms) == 0 {
		return ""
	}
	seen := make(map[string]bool, len(terms))
	parts := make([]string, 0, len(terms))
	for _, t := range terms {
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		parts = append(parts, quoteLiteral(t))
		if len(parts) >= MaxQueryTerms {
			break
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " OR ")
}

// quoteLiteral 把词元包成带转义的 FTS5 字符串字面量。
func quoteLiteral(tok string) string {
	return `"` + strings.ReplaceAll(tok, `"`, `""`) + `"`
}

// TrigramMatchExpr uses raw substrings, independently of bigram preprocessing.
// Every term is quoted; punctuation and FTS operators in user input stay data.
func TrigramMatchExpr(text string) string {
	var terms []string
	var run []rune
	flush := func() {
		for i := 0; i+3 <= len(run) && len(terms) < MaxQueryTerms; i++ {
			terms = append(terms, string(run[i:i+3]))
		}
		run = run[:0]
	}
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			run = append(run, r)
		} else {
			flush()
		}
		if len(terms) >= MaxQueryTerms {
			break
		}
	}
	flush()
	return MatchExprOf(terms)
}
