package search

import "strings"

// ConceptTerms is a small, deterministic vocabulary bridge for ordinary
// Chinese object descriptions. It adds candidates, never facts. Entity aliases
// remain the preferred story-specific vocabulary. No model is called here.
// Every component of a rule must match; a single common character is not enough.
func ConceptTerms(text string) []string {
	rules := []struct {
		term   string
		groups [][]string
	}{
		{"钥匙", [][]string{{"门匙", "钥匙", "开锁", "解锁", "把锁打开", "开启锁"}}},
		{"怀表", [][]string{{"时辰", "时间", "几点", "钟点", "时刻"}, {"看", "报", "读", "随身", "口袋"}}},
		{"账本", [][]string{{"收入", "支出", "收支", "进出", "进账", "出入"}, {"记", "册", "簿", "本", "抄"}}},
		{"茶杯", [][]string{{"喝", "饮", "盛", "装"}, {"茶"}}},
		{"罗盘", [][]string{{"方向", "方位", "东南西北", "朝向"}, {"辨", "指", "定", "认", "测"}}},
		{"灯罩", [][]string{{"光", "灯"}, {"挡", "遮", "罩", "柔"}}},
		{"地图", [][]string{{"路线", "道路", "地形", "地点", "路"}, {"标", "图", "绘", "画", "卷"}}},
		{"铃铛", [][]string{{"响", "声", "叮", "鸣"}, {"钟", "金属", "铃"}}},
		{"雨伞", [][]string{{"雨"}, {"遮", "挡", "淋", "撑"}}},
		{"绷带", [][]string{{"伤口", "伤处", "伤"}, {"包扎", "缠", "止血", "布"}}},
		{"药剂", [][]string{{"治疗", "疗伤", "止痛", "退烧"}, {"药", "瓶", "液"}}},
		{"火柴", [][]string{{"点火", "生火", "燃火"}, {"划", "木棒", "木条", "擦"}}},
		{"望远镜", [][]string{{"远处", "远方", "远距离"}, {"看", "观察", "窥"}, {"镜", "筒", "仪器"}}},
		{"车票", [][]string{{"乘车", "坐车", "上车", "列车"}, {"凭证", "票", "凭据"}}},
		{"船票", [][]string{{"乘船", "坐船", "登船"}, {"凭证", "票", "凭据"}}},
		{"信件", [][]string{{"书信", "来信", "信笺", "寄来的信"}}},
		{"水壶", [][]string{{"饮水", "喝水", "装水"}, {"壶", "瓶", "容器"}}},
		{"背包", [][]string{{"背着", "背在", "肩上"}, {"包", "行囊"}}},
		{"围巾", [][]string{{"脖子", "颈"}, {"围", "保暖", "布"}}},
		{"手套", [][]string{{"手"}, {"戴", "保暖"}, {"套", "皮革", "毛线"}}},
	}
	text = strings.ToLower(text)
	var out []string
	for _, rule := range rules {
		matched := true
		for _, alternatives := range rule.groups {
			group := false
			for _, part := range alternatives {
				if strings.Contains(text, part) {
					group = true
					break
				}
			}
			if !group {
				matched = false
				break
			}
		}
		if matched && !strings.Contains(text, rule.term) {
			out = append(out, rule.term)
		}
	}
	return out
}
