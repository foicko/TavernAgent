package application

import (
	"context"
	"encoding/json"
	"fmt"

	"tavernagent/internal/domain"
	"tavernagent/internal/util/id"
)

// 本文件是技术契约 §5.2 的编排部分：准备阶段掷骰、保存收据、提交时结算。
// 判定本身在 domain.EvaluateCheck（纯函数），这里只负责"什么时候掷、
// 掷完存哪、什么时候生效"。

// PlayerCharacterID 是玩家在状态投影里的角色 ID（与会话建立时一致）。
const PlayerCharacterID = "player"

// PrepareCheckRequest 是一次检定的准备请求。
type PrepareCheckRequest struct {
	// ActionID 必须在规则包里登记过；未登记即未授权，不执行。
	ActionID string
	// TurnID 是本回合的 ID：收据必须挂到回合上，提交结算时按回合取。
	TurnID string
	// ActorID 是执行检定的角色；为空时按玩家处理。
	ActorID string
	// Recheck is an explicit user request for fresh randomness.
	Recheck     bool
	ReuseRollID string
}

// PrepareCheck 在准备阶段为一次检定掷骰并保存收据。
//
// 三条契约语义在这里落地：
//
//  1. **rollId 复用**：rollId 是 (基准父节点, 动作, 规则版本) 的确定性函数，
//     因此同一个行动实例在同一基准上再次被请求（= 文字重生成）时，查得到
//     既有收据就**复用结果、不重掷**——只有玩家明确发起新检定才有新随机。
//  2. **独立收据行**：即使复用结果，也为本次 turnId 新建一条收据，
//     不把旧请求改成未提交（契约明确要求）。
//  3. **准备阶段不结算**：不扣物品、不推进剧情；那些在提交事务里做。
//     "准备动作不提交物品扣减，后续原子事务才结算"。
//
// 返回 prepared 状态的收据；调用方应把它交给编译层，让模型**演绎已经定下的
// 结果**，而不是让模型自己编一个结果。
func (s *TurnService) PrepareCheck(ctx context.Context, sessionID, branchID, baseHeadID string, req PrepareCheckRequest) (*domain.ActionReceipt, error) {
	s.checkMu.Lock()
	defer s.checkMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	branch, err := s.store.GetBranch(branchID)
	if err != nil || branch.SessionID != sessionID {
		return nil, Err("BRANCH_SESSION_MISMATCH", "检定分支不属于会话", 400)
	}
	node, err := s.store.GetNode(baseHeadID)
	if err != nil || node.SessionID != sessionID {
		return nil, Err("NODE_NOT_FOUND", "检定基准不属于会话", 404)
	}
	if ok, err := s.store.IsAncestor(baseHeadID, branch.HeadNodeID); err != nil {
		return nil, err
	} else if !ok {
		return nil, Err("HEAD_CONFLICT", "检定基准不在该分支路径", 409)
	}
	rules, err := s.sessionRules(sessionID)
	if err != nil {
		return nil, err
	}
	rule, ok := rules.Lookup(req.ActionID)
	if !ok {
		return nil, Err("ACTION_UNAUTHORIZED", "动作未授权: "+req.ActionID, 403)
	}
	actor := req.ActorID
	if actor == "" {
		actor = PlayerCharacterID
	}

	state, err := s.stateAt(baseHeadID)
	if err != nil {
		return nil, err
	}

	rollID := domain.RollID(baseHeadID, req.ActionID, rules.Version)
	if req.Recheck {
		rollID = domain.RollID(baseHeadID, req.ActionID+":"+req.TurnID, rules.Version)
	} else if req.ReuseRollID != "" {
		rollID = req.ReuseRollID
	}
	return s.store.PrepareReceipt(ctx, &domain.ActionReceipt{
		ReceiptID: id.New(), TurnID: req.TurnID, ActionID: req.ActionID,
		RollID: rollID, BaseHeadID: baseHeadID, RulesetVersion: rules.Version, Status: domain.ReceiptPrepared,
	}, func() (string, error) {
		if req.ReuseRollID != "" && !req.Recheck {
			return "", Err("RULE_INVALID", "原检定收据缺失，不能静默重新掷骰", 409)
		}
		if rule.Requires != nil {
			ok, err := rule.Requires.Eval(ruleContextOf(state))
			if err != nil {
				return "", Err("RULE_INVALID", "规则求值失败: "+err.Error(), 500)
			}
			if !ok {
				return "", Err("ACTION_PRECONDITION", "不满足前置条件: "+req.ActionID, 409)
			}
		}
		result, err := domain.EvaluateCheck(rule, attributeOf(state, actor, rule.Attribute), s.rand.Intn(20)+1)
		if err != nil {
			return "", err
		}
		result.RollID, result.RulesetVer = rollID, rules.Version
		if _, err := validatedRuleEvents(state, []domain.CheckResult{result}, rules.Version); err != nil {
			return "", Err("RULE_INVALID", err.Error(), 422)
		}
		blob, err := json.Marshal(result)
		return string(blob), err
	})
}

// stateAt 读取某节点的状态投影。
func (s *TurnService) stateAt(nodeID string) (*domain.WorldState, error) {
	snap, err := s.store.StateAt(nodeID)
	if err != nil || snap == nil {
		return nil, Err("STATE_MISSING", "基准状态缺失", 500)
	}
	state, err := domain.UnmarshalWorld(snap.StateJSON)
	if err != nil {
		return nil, Err("STATE_CORRUPT", err.Error(), 500)
	}
	return state, nil
}

// attributeOf 取角色的属性值；未记录时用契约基准值（修正 0 = 未经训练）。
func attributeOf(state *domain.WorldState, characterID, attribute string) int {
	if state == nil || attribute == "" {
		return domain.DefaultAttribute
	}
	ch, ok := state.Characters[characterID]
	if !ok {
		return domain.DefaultAttribute
	}
	v, ok := ch.Attributes[attribute]
	if !ok {
		return domain.DefaultAttribute
	}
	return v
}

// ruleContextOf 把世界状态投影成规则能看到的窄视图。
//
// 规则层不认识 WorldState 的结构（见 domain.RuleContext），
// 世界状态演进不会牵动规则求值——新增字段也不影响已有规则。
func ruleContextOf(state *domain.WorldState) domain.RuleContext {
	if state == nil {
		return domain.RuleContext{}
	}
	return domain.RuleContext{
		Relation: func(characterID, field string) int {
			rv, ok := state.Relationships[characterID]
			if !ok {
				return 0
			}
			switch domain.RelationshipField(field) {
			case domain.FieldAffection:
				return rv.Affection
			case domain.FieldTrust:
				return rv.Trust
			case domain.FieldAlertness:
				return rv.Alertness
			}
			return 0
		},
		Attribute: func(characterID, attribute string) int {
			ch, ok := state.Characters[characterID]
			if !ok {
				return 0
			}
			return ch.Attributes[attribute]
		},
		HasItem: func(itemID string) bool {
			item, ok := state.Items[itemID]
			return ok && item.Quantity > 0 && item.OwnerID == PlayerCharacterID
		},
		HasMilestone: func(milestoneID string) bool {
			_, ok := state.Milestones[milestoneID]
			return ok
		},
		HasSecret: func(secretID string) bool {
			for _, s := range state.UnlockedSecrets {
				if s == secretID {
					return true
				}
			}
			return false
		},
		SceneLocation: sceneLocationOf(state),
	}
}

func sceneLocationOf(state *domain.WorldState) string {
	if state.Scene == nil {
		return ""
	}
	return state.Scene.LocationID
}

// preparedChecks fails closed: missing or corrupt rulings cannot turn an
// already-decided action into a free narrative or a fresh roll.
func (s *TurnService) preparedChecks(turn *domain.TurnRequest) ([]domain.CheckResult, []string, error) {
	receipts, err := s.store.GetReceipts(turn.TurnID)
	if err != nil {
		return nil, nil, err
	}
	checks := []domain.CheckResult{}
	ids := []string{}
	for _, r := range receipts {
		if r.BaseHeadID != turn.ExpectedHeadID || r.RulesetVersion != rulesetOf(turn) || r.Status == domain.ReceiptAbandoned {
			return nil, nil, fmt.Errorf("receipt is incompatible with turn")
		}
		cr, err := r.CheckResultOf()
		if err != nil || cr.ActionID == "" || cr.RollID != r.RollID {
			return nil, nil, fmt.Errorf("invalid persisted check")
		}
		checks = append(checks, cr)
		ids = append(ids, r.ReceiptID)
	}
	return checks, ids, nil
}
