package reward

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/builtin/log"
	"go.uber.org/zap"

	"github.com/filecoin-project/specs-actors/actors/util/smoothing"

	abi "github.com/filecoin-project/specs-actors/actors/abi"
	big "github.com/filecoin-project/specs-actors/actors/abi/big"
	builtin "github.com/filecoin-project/specs-actors/actors/builtin"
	vmr "github.com/filecoin-project/specs-actors/actors/runtime"
	exitcode "github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	. "github.com/filecoin-project/specs-actors/actors/util"
	adt "github.com/filecoin-project/specs-actors/actors/util/adt"
)

type Actor struct{}

func (a Actor) Exports() []interface{} {
	return []interface{}{
		builtin.MethodConstructor: a.Constructor,
		2:                         a.AwardBlockReward,
		3:                         a.ThisEpochReward,
		4:                         a.UpdateNetworkKPI,
	}
}

var _ abi.Invokee = Actor{}

func (a Actor) Constructor(rt vmr.Runtime, currRealizedPower *abi.StoragePower) *adt.EmptyValue {
	rt.ValidateImmediateCallerIs(builtin.SystemActorAddr)

	if currRealizedPower == nil {
		rt.Abortf(exitcode.ErrIllegalArgument, "arugment should not be nil")
		return nil // linter does not understand abort exiting
	}
	st := ConstructState(*currRealizedPower)
	rt.State().Create(st)
	return nil
}

type AwardBlockRewardParams struct {
	Miner     address.Address
	Penalty   abi.TokenAmount // penalty for including bad messages in a block, >= 0
	GasReward abi.TokenAmount // gas reward from all gas fees in a block, >= 0
	WinCount  int64           // number of reward units won, > 0
}

// Awards a reward to a block producer.
// This method is called only by the system actor, implicitly, as the last message in the evaluation of a block.
// The system actor thus computes the parameters and attached value.
//
// The reward includes two components:
// - the epoch block reward, computed and paid from the reward actor's balance,
// - the block gas reward, expected to be transferred to the reward actor with this invocation.
//
// The reward is reduced before the residual is credited to the block producer, by:
// - a penalty amount, provided as a parameter, which is burnt,
func (a Actor) AwardBlockReward(rt vmr.Runtime, params *AwardBlockRewardParams) *adt.EmptyValue {
	rt.ValidateImmediateCallerIs(builtin.SystemActorAddr)
	priorBalance := rt.CurrentBalance()
	if params.Penalty.LessThan(big.Zero()) {
		rt.Abortf(exitcode.ErrIllegalArgument, "negative penalty %v", params.Penalty)
	}
	if params.GasReward.LessThan(big.Zero()) {
		rt.Abortf(exitcode.ErrIllegalArgument, "negative gas reward %v", params.GasReward)
	}
	if priorBalance.LessThan(params.GasReward) {
		rt.Abortf(exitcode.ErrIllegalState, "actor current balance %v insufficient to pay gas reward %v",
			priorBalance, params.GasReward)
	}
	if params.WinCount <= 0 {
		rt.Abortf(exitcode.ErrIllegalArgument, "invalid win count %d", params.WinCount)
	}

	minerAddr, ok := rt.ResolveAddress(params.Miner)
	if !ok {
		rt.Abortf(exitcode.ErrNotFound, "failed to resolve given owner address")
	}

	penalty := abi.NewTokenAmount(0)
	totalReward := big.Zero()
	var st State
	rt.State().Transaction(&st, func() {
		blockReward := big.Mul(st.ThisEpochReward, big.NewInt(params.WinCount))
		blockReward = big.Div(blockReward, big.NewInt(builtin.ExpectedLeadersPerEpoch))
		totalReward = big.Add(blockReward, params.GasReward)
		currBalance := rt.CurrentBalance()
		if totalReward.GreaterThan(currBalance) {
			rt.Log(vmr.WARN, "reward actor balance %d below totalReward expected %d, paying out rest of balance", currBalance, totalReward)
			totalReward = currBalance

			blockReward = big.Sub(totalReward, params.GasReward)
			// Since we have already asserted the balance is greater than gas reward blockReward is >= 0
			AssertMsg(blockReward.GreaterThanEqual(big.Zero()), "programming error, block reward is %v below zero", blockReward)
		}
		st.TotalMined = big.Add(st.TotalMined, blockReward)
	})

	// Cap the penalty at the total reward value.
	penalty = big.Min(params.Penalty, totalReward)

	// Reduce the payable reward by the penalty.
	rewardPayable := big.Sub(totalReward, penalty)

	AssertMsg(big.Add(rewardPayable, penalty).LessThanEqual(priorBalance),
		"reward payable %v + penalty %v exceeds balance %v", rewardPayable, penalty, priorBalance)

	// if this fails, we can assume the miner is responsible and avoid failing here.
	_, code := rt.Send(minerAddr, builtin.MethodsMiner.AddLockedFund, &rewardPayable, rewardPayable)
	if !code.IsSuccess() {
		rt.Log(vmr.ERROR, "failed to send AddLockedFund call to the miner actor with funds: %v, code: %v", rewardPayable, code)
		_, code := rt.Send(builtin.BurntFundsActorAddr, builtin.MethodSend, nil, rewardPayable)
		if !code.IsSuccess() {
			rt.Log(vmr.ERROR, "failed to send unsent reward to the burnt funds actor, code: %v", code)
		}
	}

	// Burn the penalty amount.
	if penalty.GreaterThan(abi.NewTokenAmount(0)) {
		_, code = rt.Send(builtin.BurntFundsActorAddr, builtin.MethodSend, nil, penalty)
		builtin.RequireSuccess(rt, code, "failed to send penalty to burnt funds actor")
	}

	return nil
}

type ThisEpochRewardReturn struct {
	ThisEpochRewardSmoothed *smoothing.FilterEstimate
	ThisEpochBaselinePower  abi.StoragePower
}

// The award value used for the current epoch, updated at the end of an epoch
// through cron tick.  In the case previous epochs were null blocks this
// is the reward value as calculated at the last non-null epoch.
func (a Actor) ThisEpochReward(rt vmr.Runtime, _ *adt.EmptyValue) *ThisEpochRewardReturn {
	rt.ValidateImmediateCallerAcceptAny()

	var st State
	rt.State().Readonly(&st)
	return &ThisEpochRewardReturn{
		ThisEpochRewardSmoothed: st.ThisEpochRewardSmoothed,
		ThisEpochBaselinePower:  st.ThisEpochBaselinePower,
	}
}

// Called at the end of each epoch by the power actor (in turn by its cron hook).
// This is only invoked for non-empty tipsets, but catches up any number of null
// epochs to compute the next epoch reward.
func (a Actor) UpdateNetworkKPI(rt vmr.Runtime, currRealizedPower *abi.StoragePower) *adt.EmptyValue {
	log.Log.Info("UpdateNetworkKPI",zap.Any("epoch",rt.CurrEpoch()),zap.Any("currRealizedPower",*currRealizedPower))
//	rt.ValidateImmediateCallerIs(builtin.StoragePowerActorAddr)
	if currRealizedPower == nil {
		rt.Abortf(exitcode.ErrIllegalArgument, "arugment should not be nil")
	}

	var st State
	rt.State().Transaction(&st, func() {
		prev := st.Epoch
		// if there were null runs catch up the computation until
		// st.Epoch == rt.CurrEpoch()
		log.Log.Info("UpdateNetworkKPI",zap.Any("stEpoch",st.Epoch))
		st.Print()
		for st.Epoch < rt.CurrEpoch() {
			// Update to next epoch to process null rounds
			st.updateToNextEpoch(*currRealizedPower)
		}

		st.updateToNextEpochWithReward(*currRealizedPower)
		log.Log.Info("the status after UpdateNetworkKPI")
		st.Print()
		log.Log.Info("~~~~~~~~~~~~~~~~~~~~~")
		// only update smoothed estimates after updating reward and epoch
		st.updateSmoothedEstimates(st.Epoch - prev)
	})
	log.Log.Info("*************************")
	return nil
}
