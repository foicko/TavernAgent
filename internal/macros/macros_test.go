package macros

import "testing"

func TestExpandCharAndUser(t *testing.T) {
	cases := []struct {
		name       string
		text       string
		char, user string
		want       string
	}{
		{"基本替换", "{{char}}从不离开{{user}}。", "玛丽", "旅人", "玛丽从不离开旅人。"},
		{"大小写与空格不敏感", "{{ char }}与{{ USER }}", "玛丽", "旅人", "玛丽与旅人"},
		{"无宏原样返回", "普通文本", "玛丽", "旅人", "普通文本"},
		{"空文本", "", "玛丽", "旅人", ""},
		{"未知宏不动", "{{system}}", "玛丽", "旅人", "{{system}}"},
		{"多次出现", "{{user}}看{{user}}", "玛丽", "旅人", "旅人看旅人"},
	}
	for _, tc := range cases {
		if got := Expand(tc.text, tc.char, tc.user); got != tc.want {
			t.Errorf("%s: Expand(%q) = %q, want %q", tc.name, tc.text, got, tc.want)
		}
	}
}
