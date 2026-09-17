package application

import "encoding/json"

func acceptMode(mode string) string {
	if mode == "" {
		return "structured"
	}
	return mode
}

// Bind a request to both its input and execution intent. New dice, another
// source node and a different mode must never alias an earlier operation.
func acceptPayloadHash(req *TurnAcceptRequest) string {
	raw, _ := json.Marshal(struct {
		Input                             any
		Mode, Base, DeriveNode, ReuseRoll string
		Recheck                           bool
	}{req.Input, acceptMode(req.Mode), req.ExpectedHeadID, req.DeriveNodeID, req.ReuseRollID, req.Recheck})
	return hashString(string(raw))
}
