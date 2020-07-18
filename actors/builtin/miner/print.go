package miner

import (
	"bytes"
	"encoding/json"
	"github.com/filecoin-project/specs-actors/tools/dlog/actorlog"
	"go.uber.org/zap"
)

func (p *SectorPreCommitInfo) Print() {
	actorlog.L.Info("the SectorPreCommitInfo params is:")
	byte,err := json.Marshal(p)
	if err != nil{
		actorlog.L.Error("SectorPreCommitInfo Print json marshal error:",zap.String("error",err.Error()))
	}
	var out bytes.Buffer
	json.Indent(&out, byte, "", "\t")
	actorlog.L.Info(out.String())
	actorlog.L.Info("")
}
