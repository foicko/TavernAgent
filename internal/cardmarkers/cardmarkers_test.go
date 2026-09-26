package cardmarkers

import (
	"reflect"
	"testing"
)

func TestExtractStripsStateMarkers(t *testing.T) {
	text := "她抬起头。[SCENE:教堂] 窗外的雪停了。[HUD:理智=82|体力=40]"
	clean, hints := Extract(text)
	// 标记左侧无空白：剥离后不留空格（中文正文不该凭空多出空格）。
	if clean != "她抬起头。窗外的雪停了。" {
		t.Errorf("clean = %q", clean)
	}
	if len(hints) != 2 {
		t.Fatalf("hints = %+v", hints)
	}
	if hints[0].Kind != KindScene || hints[0].Text != "教堂" {
		t.Errorf("scene hint = %+v", hints[0])
	}
	if hints[1].Kind != KindHUD || hints[1].Values["理智"] != "82" || hints[1].Values["体力"] != "40" {
		t.Errorf("hud hint = %+v", hints[1])
	}
}

func TestExtractKeepsPresentationalMarkers(t *testing.T) {
	text := `她笑了。<img cmd="smile"> [IMG:wave]`
	clean, hints := Extract(text)
	if clean != text {
		t.Errorf("presentational markers must stay: %q", clean)
	}
	if len(hints) != 0 {
		t.Errorf("unexpected hints: %+v", hints)
	}
}

func TestExtractEscapedBracket(t *testing.T) {
	// 转义原样保留在落库文本里，只在渲染时还原一次。
	clean, hints := Extract("数组写作 [[HUD:x] 形式")
	if clean != "数组写作 [[HUD:x] 形式" {
		t.Errorf("clean = %q", clean)
	}
	if len(hints) != 0 {
		t.Errorf("escaped marker must not be parsed: %+v", hints)
	}
}

// 提交时 Extract、读取时 SplitImages：转义的图片标记走完整条管线后仍是字面文本。
func TestEscapedImageSurvivesCommitThenSplit(t *testing.T) {
	stored, _ := Extract("写作 [[IMG:x] 形式[HUD:hp=3]")
	segs := SplitImages(stored)
	if len(segs) != 1 || segs[0].Type != "text" || segs[0].Text != "写作 [IMG:x] 形式" {
		t.Fatalf("stored=%q segs=%+v", stored, segs)
	}
}

// 剥离标记只收拾它两侧的空白，正文其余位置的缩进必须保留。
func TestExtractPreservesIndentation(t *testing.T) {
	in := "- a\n  - nested [HUD:hp=3]\n    code  x"
	clean, hints := Extract(in)
	if len(hints) != 1 {
		t.Fatalf("hints = %+v", hints)
	}
	if want := "- a\n  - nested\n    code  x"; clean != want {
		t.Errorf("clean = %q, want %q", clean, want)
	}
	if clean, _ := Extract("左 [SCENE:酒馆] 右"); clean != "左 右" {
		t.Errorf("两侧空白应合并为一个空格: %q", clean)
	}
	if clean, _ := Extract("左[SCENE:酒馆]右"); clean != "左右" {
		t.Errorf("两侧无空白时不应插入空格: %q", clean)
	}
}

func TestExtractCaseInsensitiveTag(t *testing.T) {
	clean, hints := Extract("[hud:hp=10]")
	if clean != "" || len(hints) != 1 || hints[0].Kind != KindHUD {
		t.Errorf("clean=%q hints=%+v", clean, hints)
	}
}

func TestExtractIgnoresMalformed(t *testing.T) {
	for _, s := range []string{"[HUD:noequals]", "[HUD:]", "[SCENE:]", "[OTHER:x]", "[HUD:a=b|c=d"} {
		clean, hints := Extract(s)
		if clean != s || len(hints) != 0 {
			t.Errorf("input %q -> clean=%q hints=%+v", s, clean, hints)
		}
	}
}

func TestExtractMultipleHudAndVar(t *testing.T) {
	clean, hints := Extract("[HUD:爱=1][VAR:lang=KO|num=2][HUD:爱=3]")
	if clean != "" {
		t.Errorf("clean = %q", clean)
	}
	if len(hints) != 3 {
		t.Fatalf("hints = %+v", hints)
	}
	if hints[1].Kind != KindVar || hints[1].Values["lang"] != "KO" {
		t.Errorf("var hint = %+v", hints[1])
	}
}

func TestExtractBlocks(t *testing.T) {
	cleans, hints := ExtractBlocks([]string{"A[SCENE:x]", "B", "C[HUD:a=b]"})
	if !reflect.DeepEqual(cleans, []string{"A", "B", "C"}) {
		t.Errorf("cleans = %v", cleans)
	}
	if len(hints) != 2 {
		t.Errorf("hints = %+v", hints)
	}
}

func TestExtractPreservesWhitespaceWithoutMarkers(t *testing.T) {
	// 含普通方括号但没有可解析标记：不得折叠空白或改动文本。
	text := "引用 [1]  留空  \t间隙"
	clean, hints := Extract(text)
	if clean != text || hints != nil {
		t.Errorf("clean=%q hints=%+v", clean, hints)
	}
}

func TestExtractNoMarkersFastPath(t *testing.T) {
	text := "没有任何标记的普通正文。"
	clean, hints := Extract(text)
	if clean != text || hints != nil {
		t.Errorf("clean=%q hints=%+v", clean, hints)
	}
}
