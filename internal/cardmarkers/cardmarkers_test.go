package cardmarkers

import (
	"reflect"
	"testing"
)

func TestExtractStripsStateMarkers(t *testing.T) {
	text := "她抬起头。[SCENE:教堂] 窗外的雪停了。[HUD:理智=82|体力=40]"
	clean, hints := Extract(text)
	if clean != "她抬起头。 窗外的雪停了。" {
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
	clean, hints := Extract("数组写作 [[HUD:x] 形式")
	if clean != "数组写作 [HUD:x] 形式" {
		t.Errorf("clean = %q", clean)
	}
	if len(hints) != 0 {
		t.Errorf("escaped marker must not be parsed: %+v", hints)
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

func TestExtractNoMarkersFastPath(t *testing.T) {
	text := "没有任何标记的普通正文。"
	clean, hints := Extract(text)
	if clean != text || hints != nil {
		t.Errorf("clean=%q hints=%+v", clean, hints)
	}
}
