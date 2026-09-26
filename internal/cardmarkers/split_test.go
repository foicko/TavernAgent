package cardmarkers

import "testing"

// SplitImages 是表现类标记的唯一实现：这些用例同时充当契约文档——
// 渲染端（Web / 桌面）不再有第二份语法实现，边界行为只能在这里定义。
func TestSplitImages(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []Segment
	}{
		{
			name: "无标记时返回单个文本段",
			in:   "她抬头看了你一眼。",
			want: []Segment{{Type: "text", Text: "她抬头看了你一眼。"}},
		},
		{
			name: "方括号形式切段",
			in:   "前[IMG:emo_smile]后",
			want: []Segment{
				{Type: "text", Text: "前"},
				{Type: "image", AssetKey: "emo_smile"},
				{Type: "text", Text: "后"},
			},
		},
		{
			name: "标签形式切段（含自闭合与大小写）",
			in:   `A<img cmd="emo_cry"/>B<IMG CMD="emo_x">C`,
			want: []Segment{
				{Type: "text", Text: "A"},
				{Type: "image", AssetKey: "emo_cry"},
				{Type: "text", Text: "B"},
				{Type: "image", AssetKey: "emo_x"},
				{Type: "text", Text: "C"},
			},
		},
		{
			name: "兼容 RisuAI 多种图像指令写法",
			in:   `<img="a"> <img_obj="b"> <imgBG="c"> <img src="d" relation="Sister" favorability="20">`,
			want: []Segment{
				{Type: "image", AssetKey: "a"},
				{Type: "text", Text: " "},
				{Type: "image", AssetKey: "b"},
				{Type: "text", Text: " "},
				{Type: "image", AssetKey: "c"},
				{Type: "text", Text: " "},
				{Type: "image", AssetKey: "d"},
			},
		},
		{
			name: "空键不构成图片：原样留在文本里",
			in:   "前[IMG:]后",
			want: []Segment{{Type: "text", Text: "前[IMG:]后"}},
		},
		{
			name: "转义的双方括号是字面量",
			in:   "写作 [[IMG:x] 形式",
			want: []Segment{{Type: "text", Text: "写作 [IMG:x] 形式"}},
		},
		{
			name: "键两侧空白被裁剪",
			in:   "[IMG:  emo_smile  ]",
			want: []Segment{{Type: "image", AssetKey: "emo_smile"}},
		},
		{
			name: "空输入返回 nil",
			in:   "",
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SplitImages(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("段数 = %d, 期望 %d (%+v)", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("段 %d = %+v, 期望 %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// 文本段拼接必须能还原原文（除转义还原外不能丢字符）：
// 分段是投影，不是改写。
func TestSplitImagesPreservesText(t *testing.T) {
	in := "第一段\n[IMG:a]第二段 <img cmd=\"b\">  第三段"
	got := SplitImages(in)
	var joined string
	for _, seg := range got {
		if seg.Type == "text" {
			joined += seg.Text
		}
	}
	if joined != "第一段\n第二段   第三段" {
		t.Errorf("文本段拼接 = %q", joined)
	}
}
