package context_test

import (
	"strings"
	"testing"

	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
)

// 常驻条目（Constant）不依赖关键词命中，且未命中任何关键词时也应注入。
func TestLorebookInjectsConstantWithoutKeyword(t *testing.T) {
	f := newFixture(t, book(
		domain.LorebookEntry{EntryID: "c1", Content: "常驻设定：钟楼由旧教会建造。", Enabled: true, Constant: true},
		entry("e1", true, "普通设定只有命中才出现。", "看护人"),
	), ctxpkg.DefaultOptions())

	got := f.systemPrompt(t, testRootID, "我在路边买了两个苹果。")
	if !strings.Contains(got, "常驻设定") {
		t.Fatalf("常驻条目未注入:\n%s", got)
	}
	if strings.Contains(got, "普通设定只有命中") {
		t.Fatalf("未命中的普通条目被注入:\n%s", got)
	}
}

// 禁用的常驻条目仍不应注入。
func TestLorebookSkipsDisabledConstant(t *testing.T) {
	f := newFixture(t, book(
		domain.LorebookEntry{EntryID: "c1", Content: "被禁用的常驻内容。", Enabled: false, Constant: true},
	), ctxpkg.DefaultOptions())

	got := f.systemPrompt(t, testRootID, "任意输入。")
	if strings.Contains(got, "被禁用的常驻内容") {
		t.Fatalf("禁用的常驻条目被注入:\n%s", got)
	}
}

// Order 决定注入顺序（数值小者先），与声明顺序无关。
func TestLorebookHonorsOrder(t *testing.T) {
	f := newFixture(t, book(
		domain.LorebookEntry{EntryID: "late", Keys: []string{"灯塔"}, Content: "后注入的条目。", Enabled: true, Order: 5},
		domain.LorebookEntry{EntryID: "early", Keys: []string{"灯塔"}, Content: "先注入的条目。", Enabled: true, Order: 1},
	), ctxpkg.DefaultOptions())

	got := f.systemPrompt(t, testRootID, "我走向灯塔。")
	iEarly := strings.Index(got, "先注入的条目")
	iLate := strings.Index(got, "后注入的条目")
	if iEarly < 0 || iLate < 0 || iEarly > iLate {
		t.Fatalf("Order 未生效（early=%d late=%d）:\n%s", iEarly, iLate, got)
	}
}

// 预算紧张时，常驻条目优先于关键词命中，保证核心设定不被挤出。
func TestLorebookConstantWinsBudget(t *testing.T) {
	opts := ctxpkg.DefaultOptions()
	opts.MaxLorebookEntries = 1
	f := newFixture(t, book(
		entry("e1", true, "关键词命中的普通条目。", "灯塔"),
		domain.LorebookEntry{EntryID: "c1", Content: "常驻的核心设定。", Enabled: true, Constant: true},
	), opts)

	got := f.systemPrompt(t, testRootID, "灯塔就在前方。")
	if !strings.Contains(got, "常驻的核心设定") {
		t.Fatalf("常驻条目应在预算内优先注入:\n%s", got)
	}
	if strings.Contains(got, "关键词命中的普通条目") {
		t.Fatalf("预算已满时普通命中应被让位:\n%s", got)
	}
}
