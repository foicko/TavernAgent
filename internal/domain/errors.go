package domain

import "errors"

// 领域错误：应用层据此返回明确的 HTTP 错误码（技术契约 §11.2）。
var (
	ErrItemNotFound           = errors.New("item not found")
	ErrItemNotOwned           = errors.New("item not owned by actor")
	ErrItemInsufficientQty    = errors.New("insufficient item quantity")
	ErrItemAlreadyExists      = errors.New("item already exists")
	ErrKeepsakeLimit          = errors.New("keepsake slot limit reached")
	ErrRelationshipOutOfRange = errors.New("relationship value out of range")
	ErrUnknownField           = errors.New("unknown relationship field")
	ErrPromiseExists          = errors.New("promise already exists")
	ErrPromiseSettled         = errors.New("promise already settled")
	ErrUnknownEvent           = errors.New("event has no applier")
	ErrEventReplay            = errors.New("event application failed during replay")
)
