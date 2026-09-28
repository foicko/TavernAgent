package sqlite

import "strings"

// SQL IN (...) 构造的唯一入口。
//
// 此前至少五处各自手写占位符（`strings.Repeat("?,", n-1)+"?"` 或循环拼接），
// 空切片时 `strings.Repeat("?,", -1)` 会 panic、`IN ()` 是语法错误，且各处
// 参数拼装顺序不一。集中后：空切片一律返回空串与 nil，调用方必须显式跳过
// 空批次（而不是靠"恰好没人传空"）。

// placeholders 返回 n 个占位符（"?,?,?"）；n<=0 返回空串。
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("?,", n-1) + "?"
}

// inClause 返回 IN 子句的占位符与参数。空切片返回 ("", nil)，调用方必须跳过。
func inClause(ids []string) (string, []any) {
	if len(ids) == 0 {
		return "", nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return placeholders(len(ids)), args
}

// inClauseAny 是 inClause 的泛型版本，供 []int / []domain.EventType 等使用。
func inClauseAny[T any](items []T) (string, []any) {
	if len(items) == 0 {
		return "", nil
	}
	args := make([]any, len(items))
	for i, v := range items {
		args[i] = v
	}
	return placeholders(len(items)), args
}
