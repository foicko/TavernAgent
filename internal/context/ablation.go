package context

import (
	"fmt"
	"sort"
	"strings"
)

// 消融开关（ADS-7.8-01）。
//
// 为什么需要：系统里每一项"让体验更好"的特性——上下文动静分离、记忆召回、
// 世界书注入、摘要压缩——都只是**假设**有用。没有一键关闭它们的能力，就永远
// 回答不了"这个特性真的改善了结果，还是只是感觉有用"，也无法在评估里造出
// 裸模型基线来做对照。
//
// 规范强调开关必须**在启动路径的极早期注入**（在任何模块捕获配置值之前）。
// 本项目的对应做法：main 解析 -ablate 后立刻 Apply 到 CompilerOptions，
// 再由该选项构造唯一的 Compiler——不存在"构造完再改"的中间态。
//
// 语义约定：列出的特性被**关闭**（而不是打开），因此 `-ablate=all` 表示
// 关掉全部可消融特性，得到最接近裸模型的基线。开关只在启动时生效，
// 不提供运行时切换：运行中切换会让同一次评估里混入两个配置。
type Ablation struct {
	// DynamicContext 关闭「静态前缀 / 动态状态分离」，回到单条 system 合并模式。
	// 关掉它同时会失去前缀缓存收益——这正是要测的代价。
	DynamicContext bool
	// Memory 关闭记忆召回注入（已落库的记忆不受影响，只是不进上下文）。
	Memory bool
	// Lorebook 关闭世界书关键词注入。
	Lorebook bool
	// Summaries 关闭摘要的注入与折叠；与 Compaction 一起用才是"完全没有摘要"。
	Summaries bool
	// Compaction 关闭后台压缩（不再生成新摘要，但已存在的摘要仍会被注入，
	// 除非同时列出 summaries）。
	Compaction bool
}

// ablationAll 是 `all` 关键字展开后的集合。
var ablationAll = []string{"dynamic-context", "memory", "lorebook", "summaries", "compaction"}

// AblationNames 返回全部可消融特性名（供 -h 说明与校验共用）。
func AblationNames() []string {
	out := append([]string(nil), ablationAll...)
	sort.Strings(out)
	return out
}

// ParseAblation 解析逗号分隔的消融清单。
//
// 允许 `all` 与其他项混写（幂等）。空字符串表示不消融任何特性——这是默认，
// 也是生产形态。未知特性名必须报错而不是静默忽略：拼错一个名字会让评估
// 得到一个"以为关了其实没关"的配置，比直接失败更危险。
func ParseAblation(spec string) (Ablation, error) {
	var a Ablation
	for _, raw := range strings.FieldsFunc(spec, func(r rune) bool { return r == ',' || r == ' ' }) {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" {
			continue
		}
		if name == "all" {
			for _, n := range ablationAll {
				if err := a.enable(n); err != nil {
					return Ablation{}, err
				}
			}
			continue
		}
		if err := a.enable(name); err != nil {
			return Ablation{}, err
		}
	}
	return a, nil
}

func (a *Ablation) enable(name string) error {
	switch name {
	case "dynamic-context":
		a.DynamicContext = true
	case "memory":
		a.Memory = true
	case "lorebook":
		a.Lorebook = true
	case "summaries":
		a.Summaries = true
	case "compaction":
		a.Compaction = true
	default:
		return fmt.Errorf("未知消融特性 %q；可选：%s（或 all）",
			name, strings.Join(AblationNames(), ", "))
	}
	return nil
}

// Enabled 表示确实关闭了至少一项特性。
func (a Ablation) Enabled() bool {
	return a.DynamicContext || a.Memory || a.Lorebook || a.Summaries || a.Compaction
}

// Names 返回被关闭的特性名（保持稳定顺序，便于日志与报告比对）。
func (a Ablation) Names() []string {
	var out []string
	if a.Compaction {
		out = append(out, "compaction")
	}
	if a.DynamicContext {
		out = append(out, "dynamic-context")
	}
	if a.Lorebook {
		out = append(out, "lorebook")
	}
	if a.Memory {
		out = append(out, "memory")
	}
	if a.Summaries {
		out = append(out, "summaries")
	}
	return out
}

// String 返回可读描述；未启用时返回 "none"（生产形态）。
func (a Ablation) String() string {
	if !a.Enabled() {
		return "none"
	}
	return strings.Join(a.Names(), ",")
}

// Apply 把消融开关落到编译选项上。
//
// 只在启动阶段调用一次。这里刻意不提供反向开关：任何"运行中重新打开某特性"
// 的路径都会让评估结果无法归因到单一配置。
func (a Ablation) Apply(opts *CompilerOptions) {
	if opts == nil {
		return
	}
	if a.DynamicContext {
		opts.SplitDynamicContext = false
	}
	if a.Memory {
		opts.DisableMemoryInjection = true
	}
	if a.Lorebook {
		opts.DisableLorebookInjection = true
	}
	if a.Summaries {
		opts.DisableSummaries = true
	}
	if a.Compaction {
		opts.CompactionPolicy.Disabled = true
	}
}
