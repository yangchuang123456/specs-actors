package reward

import (
	"bytes"
	"encoding/json"
	"fmt"
	abi "github.com/filecoin-project/specs-actors/actors/abi"
	big "github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/tools/dlog/actorlog"
	"go.uber.org/zap"
	"log"
)

// A quantity of space * time (in byte-epochs) representing power committed to the network for some duration.
type Spacetime = big.Int

type State struct {
	// CumsumBaseline is a target CumsumRealized needs to reach for EffectiveNetworkTime to increase
	// CumsumBaseline and CumsumRealized are expressed in byte-epochs.
	CumsumBaseline Spacetime

	// CumsumRealized is cumulative sum of network power capped by BalinePower(epoch)
	CumsumRealized Spacetime

	// EffectiveNetworkTime is ceiling of real effective network time `theta` based on
	// CumsumBaselinePower(theta) == CumsumRealizedPower
	// Theta captures the notion of how much the network has progressed in its baseline
	// and in advancing network time.
	EffectiveNetworkTime abi.ChainEpoch

	// EffectiveBaselinePower is the baseline power at the EffectiveNetworkTime epoch
	EffectiveBaselinePower abi.StoragePower

	// The reward to be paid in per WinCount to block producers.
	// The actual reward total paid out depends on the number of winners in any round.
	// This value is recomputed every non-null epoch and used in the next non-null epoch.
	ThisEpochReward abi.TokenAmount

	// The baseline power the network is targeting at st.Epoch
	ThisEpochBaselinePower abi.StoragePower

	// Epoch tracks for which epoch the Reward was computed
	Epoch abi.ChainEpoch
}

func ConstructState(currRealizedPower abi.StoragePower) *State {
	actorlog.L.Info("call reward state ConstructState")
	st := &State{
		CumsumBaseline:         big.Zero(),
		CumsumRealized:         big.Zero(),
		EffectiveNetworkTime:   0,
		EffectiveBaselinePower: BaselineInitialValue,

		ThisEpochReward:        big.Zero(),
		ThisEpochBaselinePower: BaselineInitialValue,
		Epoch:                  -1,
	}

	st.updateToNextEpochWithReward(currRealizedPower)
	st.Print()

	return st
}

// Takes in current realized power and updates internal state
// Used for update of internal state during null rounds
func (st *State) updateToNextEpoch(currRealizedPower abi.StoragePower) {
	actorlog.L.Info("updateToNextEpoch start the state is:",zap.Any("stAddr",fmt.Sprintf("%p",st)))
	st.Print()
	actorlog.L.Info("updateToNextEpoch execute is:111")
	st.Epoch++
	st.ThisEpochBaselinePower = BaselinePowerNextEpoch(st.ThisEpochBaselinePower)
	actorlog.L.Info("updateToNextEpoch execute is:222")
	cappedRealizedPower := big.Min(st.ThisEpochBaselinePower, currRealizedPower)
	st.CumsumRealized = big.Add(st.CumsumRealized, cappedRealizedPower)
	actorlog.L.Info("updateToNextEpoch execute is:333",zap.Any("st.EffectiveBaselinePower",st.EffectiveBaselinePower))
	st.Print()
	for st.CumsumRealized.GreaterThan(st.CumsumBaseline) {
		actorlog.L.Info("st.CumsumRealized.GreaterThan(st.CumsumBaseline)",zap.Any("CumsumRealized", st.CumsumRealized), zap.Any("CumsumBaseline", st.CumsumBaseline),zap.Any("epoch",st.Epoch))
		st.EffectiveNetworkTime++
		actorlog.L.Info("st.CumsumRealized.GreaterThan(st.CumsumBaseline)",zap.Any("EffectiveNetworkTime", st.EffectiveNetworkTime),zap.Any("epoch",st.Epoch))
		st.EffectiveBaselinePower = BaselinePowerNextEpoch(st.EffectiveBaselinePower)
		actorlog.L.Info("st.EffectiveBaselinePower",zap.Any("st.EffectiveBaselinePower", st.EffectiveBaselinePower),zap.Any("epoch",st.Epoch))
		st.CumsumBaseline = big.Add(st.CumsumBaseline, st.EffectiveBaselinePower)
		actorlog.L.Info("st.CumsumBaseline",zap.Any("st.CumsumBaseline", st.CumsumBaseline),zap.Any("epoch",st.Epoch))
	}
	actorlog.L.Info("updateToNextEpoch end the state is:")
	st.Print()
}

// Takes in a current realized power for a reward epoch and computes
// and updates reward state to track reward for the next epoch
func (st *State) updateToNextEpochWithReward(currRealizedPower abi.StoragePower) {
	prevRewardTheta := computeRTheta(st.EffectiveNetworkTime, st.EffectiveBaselinePower, st.CumsumRealized, st.CumsumBaseline)
	st.updateToNextEpoch(currRealizedPower)
	currRewardTheta := computeRTheta(st.EffectiveNetworkTime, st.EffectiveBaselinePower, st.CumsumRealized, st.CumsumBaseline)

	st.ThisEpochReward = computeReward(st.Epoch, prevRewardTheta, currRewardTheta)
}

func (st *State) Print() {
	actorlog.L.Info("the reward state is:")
	byte, err := json.Marshal(st)
	if err != nil {
		actorlog.L.Info("reward state json marshal error:", zap.String("error", err.Error()))
	}
	var out bytes.Buffer
	json.Indent(&out, byte, "", "\t")
	actorlog.L.Info(out.String())
	log.Println(out.String())
}