package reward

import (
	"bytes"
	"encoding/json"
	abi "github.com/filecoin-project/specs-actors/actors/abi"
	big "github.com/filecoin-project/specs-actors/actors/abi/big"
	"go.uber.org/zap"
	"log"
	gbig "math/big"
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

	// The reward to be paid in per WinCount to block producers.
	// The actual reward total paid out depends on the number of winners in any round.
	ThisEpochReward abi.TokenAmount

	// Epoch tracks for which epoch the Reward was computed
	Epoch abi.ChainEpoch
}

func ConstructState(currRealizedPower abi.StoragePower) *State {
	st := &State{
		CumsumBaseline:       big.Zero(),
		CumsumRealized:       big.Zero(),
		EffectiveNetworkTime: 0,

		ThisEpochReward: big.Zero(),
		Epoch:           -1,
	}

	st.updateToNextEpochWithReward(currRealizedPower)

	return st
}

// Takes in current realized power and updates internal state
// Used for update of internal state during null rounds
func (st *State) updateToNextEpoch(currRealizedPower abi.StoragePower) {
	st.Epoch++

	cappedRealizedPower := big.Min(BaselinePowerAt(st.Epoch), currRealizedPower)
	st.CumsumRealized = big.Add(st.CumsumRealized, cappedRealizedPower)

	log.Println("updateToNextEpoch the effectiveNetworkTime, cumsumRealized,cumsumBaseline is:",st.EffectiveNetworkTime,st.CumsumRealized,st.CumsumBaseline)

	for st.CumsumRealized.GreaterThan(st.CumsumBaseline) {
		st.EffectiveNetworkTime++
		st.CumsumBaseline = big.Add(st.CumsumBaseline, BaselinePowerAt(st.EffectiveNetworkTime))
	}
}

// Takes in a current realized power for a reward epoch and computes
// and updates reward state to track reward for the next epoch
func (st *State) updateToNextEpochWithReward(currRealizedPower abi.StoragePower) {
	prevRewardTheta := computeRTheta(st.EffectiveNetworkTime, st.CumsumRealized, st.CumsumBaseline)
	st.updateToNextEpoch(currRealizedPower)
	currRewardTheta := computeRTheta(st.EffectiveNetworkTime, st.CumsumRealized, st.CumsumBaseline)

	log.Println("updateToNextEpochWithReward the prevRewardTheta and  currRewardTheta is:", Q128ToF(prevRewardTheta), Q128ToF(currRewardTheta))
	st.ThisEpochReward = computeReward(st.Epoch, prevRewardTheta, currRewardTheta)
	log.Println("updateToNextEpochWithReward ThisEpochReward is:",big.Div(st.ThisEpochReward, abi.TokenPrecision))
}

func Q128ToF(x big.Int) float64 {
	q128 := new(gbig.Int).SetInt64(1)
	q128 = q128.Lsh(q128, precision)
	res, _ := new(gbig.Rat).SetFrac(x.Int, q128).Float64()
	return res
}

func (st *State) Print() {
	log.Println("the reward state is:")
	st.ThisEpochReward = big.Div(st.ThisEpochReward, abi.TokenPrecision)
	byte, err := json.Marshal(st)
	if err != nil {
		log.Println("reward state json marshal error:", zap.String("error", err.Error()))
	}
	var out bytes.Buffer
	json.Indent(&out, byte, "", "\t")
	log.Println(out.String())
	log.Println("")
}
