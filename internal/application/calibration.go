package application

import (
	"log/slog"
	"sync"
	"time"

	"tavernagent/internal/ports"
)

// tokenCalibration 用真实用量反推 token 估算系数。
//
// 估算函数（context.EstimateTokens）是纯字符启发式，自己就写明"真实用量应由
// 供应商 usage 校准"。预算判定、尾部窗口、摘要输入预算全都建立在它上面：
// 估算偏高会提前压缩并在边缘情况误报"装不下"，偏低则会撑爆窗口。
//
// 台账里两组数字一直都在：estimated 是编译侧估算，prompt_tokens 是供应商回传
// 的真实值。比值就是系数，不需要任何新数据源。
type tokenCalibration struct {
	mu       sync.Mutex
	factor   float64
	computed time.Time
}

const (
	// calibrationMinSamples 是生效所需的最少已回传样本数（太少容易被单次
	// 长上下文样本带偏）。
	calibrationMinSamples = 8
	// calibrationRefresh 是重算间隔：系数变化缓慢，不需要每回合查询。
	calibrationRefresh = time.Minute
	// calibrationFloor/Ceil 夹住异常样本（网关回传错值、混用不同分词器）。
	calibrationFloor = 0.5
	calibrationCeil  = 2.0
	// calibrationSampleLimit 是每次重算读取的最近记录条数。
	calibrationSampleLimit = 200
)

// CalibrateFromUsage 从用量记录算出系数：sum(真实输入) / sum(估算输入)。
//
// 只统计供应商确实回传 usage、且估算值非零的记录。样本不足时返回 ok=false，
// 调用方应保持上一次的系数（初始为 1.0，即不校准）。
func CalibrateFromUsage(rows []ports.TurnUsageRecord) (float64, bool) {
	var reported, estimated int64
	samples := 0
	for _, row := range rows {
		if !row.Reported || row.Prompt <= 0 || row.Estimated <= 0 {
			continue
		}
		reported += int64(row.Prompt)
		estimated += int64(row.Estimated)
		samples++
	}
	if samples < calibrationMinSamples || estimated == 0 {
		return 0, false
	}
	factor := float64(reported) / float64(estimated)
	if factor < calibrationFloor {
		factor = calibrationFloor
	}
	if factor > calibrationCeil {
		factor = calibrationCeil
	}
	return factor, true
}

// tokenCalibrationFactor 返回当前应使用的系数（带缓存，样本不足时沿用旧值）。
func (s *TurnService) tokenCalibrationFactor() float64 {
	if s == nil {
		return 0
	}
	cal := &s.calibration
	cal.mu.Lock()
	defer cal.mu.Unlock()
	if cal.factor > 0 && time.Since(cal.computed) < calibrationRefresh {
		return cal.factor
	}
	if s.usage == nil {
		return cal.factor
	}
	rows, err := s.usage.RecentTurnUsage(calibrationSampleLimit)
	if err != nil {
		// 观测数据取不到不影响生成：沿用旧系数即可。
		return cal.factor
	}
	factor, ok := CalibrateFromUsage(rows)
	if !ok {
		return cal.factor
	}
	if cal.factor != factor {
		slog.Info("token estimate calibrated from real usage", "factor", factor, "was", cal.factor)
	}
	cal.factor = factor
	cal.computed = time.Now()
	return factor
}
