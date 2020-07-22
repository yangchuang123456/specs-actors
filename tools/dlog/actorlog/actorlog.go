package actorlog

import (
	"github.com/filecoin-project/specs-actors/tools/util"
	"go.uber.org/zap"
)

var L *zap.Logger

func init() {
	//L = util.GetXDebugLog("p2p")
	L = util.GetXDebugLog("actor")
}
