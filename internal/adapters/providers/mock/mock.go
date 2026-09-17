// Package mock 提供 Mock 模型供应商：可注入故障流用于早期集成测试（M0 未知项最小实验）。
package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"tavernagent/internal/ports"
)

// Item 是脚本中的一步。Line 按 Split 字节步长分片交付。
type Item struct {
	Line  string
	Split int   // <=0 表示整体交付
	Err   error // 该步之后流以该错误结束（模拟网络异常/截断）
}

// Provider 是可配置的 Mock 供应商。
type Provider struct {
	mu     sync.Mutex
	Script []Item
	Caps   ports.ProviderCapabilities
}

var _ ports.ModelProvider = (*Provider)(nil)

// New 创建默认能力的 Mock。
func New(script []Item) *Provider {
	return &Provider{
		Script: script,
		Caps: ports.ProviderCapabilities{
			ID: "mock", Streaming: true, StructuredOutput: true, Continuation: false,
			ContextWindow: 128000, TokenizerKnown: false,
		},
	}
}

func (p *Provider) Capabilities(ctx context.Context) (ports.ProviderCapabilities, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.Caps.ID == "" {
		p.Caps.ID = "mock"
	}
	return p.Caps, nil
}

// Stream 按脚本交付原始帧协议字节流。
//
// 用量按实际收发字节**确定性地合成**（4 字节 ≈ 1 token），使集成测试可以断言
// "usage 被采集并落库"这条链路，而不需要真实模型。Cached 恒为 0：
// mock 不模拟前缀缓存，缓存命中只能由真实供应商验证。
func (p *Provider) Stream(ctx context.Context, req ports.ChatRequest, sink ports.StreamSink) error {
	p.mu.Lock()
	items := append([]Item(nil), p.Script...)
	p.mu.Unlock()
	promptBytes := 0
	for _, m := range req.Messages {
		promptBytes += len(m.Content)
	}
	sent := 0
	for _, it := range items {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := it.Line
		if line != "" {
			if it.Split <= 0 {
				if err := sink.Chunk([]byte(line)); err != nil {
					return err
				}
				sent += len(line)
			} else {
				for _, part := range chunk(line, it.Split) {
					if err := sink.Chunk([]byte(part)); err != nil {
						return err
					}
					sent += len(part)
				}
			}
		}
		if it.Err != nil {
			return fmt.Errorf("%s: %w", req.Model, it.Err)
		}
	}
	if sent > 0 || promptBytes > 0 {
		sink.Usage(ports.TokenUsage{
			Prompt:     promptBytes / 4,
			Completion: sent / 4,
			Cached:     0,
			Reported:   true,
		})
	}
	return nil
}

// SetScript 替换脚本（供测试中途注入）。
func (p *Provider) SetScript(script []Item) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Script = script
}

func chunk(s string, n int) []string {
	if n <= 0 {
		return []string{s}
	}
	var out []string
	for len(s) > n {
		out = append(out, s[:n])
		s = s[n:]
	}
	if len(s) > 0 {
		out = append(out, s)
	}
	return out
}

// ---- 常用脚本构造 ----

// Frame 返回一行协议帧。
func Frame(json string) Item { return Item{Line: json + "\n"} }

// Fault 返回带故障的项。
func Fault(err error) Item { return Item{Err: err} }

var ErrSimulatedNetwork = fmt.Errorf("simulated network failure")
var ErrSimulatedTruncation = fmt.Errorf("simulated truncation")

// BuildLine 构造一行 JSON 帧。
func BuildLine(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

// DefaultStoryScript 返回一条可完整提交的演示回合（M0 开发体验用）。
func DefaultStoryScript() []Item {
	block1 := `{"v":1,"seq":1,"type":"block","kind":"narration","speakerId":null,"text":"雨水沿着酒馆的窗框流下，壁炉的火光在墙上摇曳。"}`
	block2 := `{"v":1,"seq":2,"type":"block","kind":"dialogue","speakerId":"npc_elena","text":"你终于回来了。还记得我们的约定吗？"}`
	proposals := `{"proposalId":"p1","type":"relationship_delta","characterId":"npc_elena","field":"trust","delta":1,"evidenceBlockSeqs":[2]},` +
		`{"proposalId":"m1","type":"memory_add","memoryKind":"observed","text":"雨水沿着酒馆窗框流下，壁炉里燃着火。","sourceQuote":"雨水沿着酒馆的窗框流下","evidenceConfidence":"high"}`
	options := `{"optionId":"o1","intent":"emotional","text":"我记得那枚怀表的事。"},{"optionId":"o2","intent":"clever","text":"让我先想想，我们约定了什么？"}`
	final := `{"v":1,"seq":3,"type":"final","proposals":[` + proposals + `],"options":[` + options + `]}`
	return []Item{
		Frame(block1),
		{Line: block2 + "\n", Split: 7},
		Frame(final),
	}
}
