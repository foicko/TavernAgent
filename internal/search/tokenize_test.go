package search

import (
	"reflect"
	"strings"
	"testing"
)

func TestTokenizeCJKBigrams(t *testing.T) {
	got := Tokenize("铜钥匙")
	want := []string{"铜钥", "钥匙"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Tokenize(铜钥匙) = %v, want %v", got, want)
	}
}

func TestTokenizeTwoCharRunHasSingleBigram(t *testing.T) {
	// 两个字只能构成一个相邻双字——这决定了「钥匙」能命中「铜钥匙」的索引。
	got := Tokenize("钥匙")
	want := []string{"钥匙"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Tokenize(钥匙) = %v, want %v", got, want)
	}
}

func TestTokenizeSingleHanProducesNothing(t *testing.T) {
	// 单字不成词元：它由实体/短查询路径兜底，不进入索引（噪声大于收益）。
	if got := Tokenize("门"); len(got) != 0 {
		t.Fatalf("单字不应产生词元，得到 %v", got)
	}
}

func TestTokenizeLatinAndDigits(t *testing.T) {
	got := Tokenize("GeoTIFF-2 和 PLY")
	want := []string{"geotiff", "2", "ply"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Tokenize = %v, want %v", got, want)
	}
}

func TestTokenizeSplitsAtPunctuation(t *testing.T) {
	// 标点断开连续串：每一段各自切双字。
	got := Tokenize("磨刀，声音。")
	want := []string{"磨刀", "声音"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Tokenize = %v, want %v", got, want)
	}
}

func TestTokenizeKeepsDuplicateForTermFrequency(t *testing.T) {
	// 索引侧需要词频（BM25 依赖 tf），因此 Tokenize 不去重。
	got := Tokenize("钥匙钥匙")
	want := []string{"钥匙", "匙钥", "钥匙"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Tokenize = %v, want %v", got, want)
	}
}

func TestTokensDeduplicates(t *testing.T) {
	got := Tokens("钥匙钥匙")
	want := []string{"钥匙", "匙钥"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Tokens = %v, want %v", got, want)
	}
}

func TestTokensCapsTermCount(t *testing.T) {
	// 用 200 个互不相同的汉字构造 199 个互不相同的相邻双字，才能真的碰到上限。
	var sb strings.Builder
	for r := rune(0x4E00); r < 0x4E00+200; r++ {
		sb.WriteRune(r)
	}
	got := Tokens(sb.String())
	if len(got) != MaxQueryTerms {
		t.Fatalf("词元数 = %d, want %d", len(got), MaxQueryTerms)
	}
}

func TestMatchExprQuotesEveryTerm(t *testing.T) {
	got := MatchExpr("钥匙")
	if got != `"钥匙"` {
		t.Fatalf("MatchExpr = %q", got)
	}
}

func TestMatchExprEscapesOperatorShapedText(t *testing.T) {
	// 正文里出现 AND 与引号时，必须被当成字符串而不是 FTS 语法。
	got := MatchExpr(`AND "钥匙"`)
	want := `"and" OR "钥匙"`
	if got != want {
		t.Fatalf("MatchExpr = %q, want %q", got, want)
	}
}

func TestMatchExprEmptyWhenNoTokens(t *testing.T) {
	if got := MatchExpr("门"); got != "" {
		t.Fatalf("单字查询应返回空表达式，得到 %q", got)
	}
	if got := MatchExpr("   ，。 "); got != "" {
		t.Fatalf("纯标点应返回空表达式，得到 %q", got)
	}
}
