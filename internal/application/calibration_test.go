package application

import (
	"testing"

	"tavernagent/internal/ports"
)

// 系数必须来自"供应商回传过 usage"的样本：未回传的记录没有真实值，
// 混进去会把系数带向 1（等于悄悄关掉校准）。
func TestCalibrateFromUsageUsesOnlyReportedSamples(t *testing.T) {
	rows := []ports.TurnUsageRecord{}
	for i := 0; i < calibrationMinSamples; i++ {
		rows = append(rows, ports.TurnUsageRecord{Reported: true, Prompt: 800, Estimated: 1000})
	}
	// 这些记录缺少真实值，必须被忽略。
	rows = append(rows,
		ports.TurnUsageRecord{Reported: false, Prompt: 0, Estimated: 9999},
		ports.TurnUsageRecord{Reported: true, Prompt: 0, Estimated: 5000},
	)
	factor, ok := CalibrateFromUsage(rows)
	if !ok {
		t.Fatal("样本充足时应给出系数")
	}
	if factor < 0.79 || factor > 0.81 {
		t.Fatalf("系数 = %v，期望约 0.8（真实 800 / 估算 1000）", factor)
	}
}

func TestCalibrateFromUsageNeedsEnoughSamples(t *testing.T) {
	rows := []ports.TurnUsageRecord{{Reported: true, Prompt: 500, Estimated: 1000}}
	if _, ok := CalibrateFromUsage(rows); ok {
		t.Fatal("样本不足时不得给出系数（应保持不校准）")
	}
}

// 异常样本（网关回传错值、混用分词器）必须被夹住，否则预算会被带偏到无法使用。
func TestCalibrateFromUsageClampsOutliers(t *testing.T) {
	var tiny, huge []ports.TurnUsageRecord
	for i := 0; i < calibrationMinSamples; i++ {
		tiny = append(tiny, ports.TurnUsageRecord{Reported: true, Prompt: 10, Estimated: 10000})
		huge = append(huge, ports.TurnUsageRecord{Reported: true, Prompt: 100000, Estimated: 1000})
	}
	if factor, ok := CalibrateFromUsage(tiny); !ok || factor != calibrationFloor {
		t.Fatalf("极小比值应夹到下限 %v，实际 %v ok=%v", calibrationFloor, factor, ok)
	}
	if factor, ok := CalibrateFromUsage(huge); !ok || factor != calibrationCeil {
		t.Fatalf("极大比值应夹到上限 %v，实际 %v ok=%v", calibrationCeil, factor, ok)
	}
}
