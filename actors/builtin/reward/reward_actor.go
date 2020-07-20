package reward

import (
	"github.com/filecoin-project/go-address"
	abi "github.com/filecoin-project/specs-actors/actors/abi"
	big "github.com/filecoin-project/specs-actors/actors/abi/big"
	builtin "github.com/filecoin-project/specs-actors/actors/builtin"
	vmr "github.com/filecoin-project/specs-actors/actors/runtime"
	exitcode "github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	. "github.com/filecoin-project/specs-actors/actors/util"
	adt "github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/filecoin-project/specs-actors/tools/dlog/actorlog"
	"go.uber.org/zap"
	"log"
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
	actorlog.L.Info("call reward actor constructor")
	rt.State().Create(st)
	return nil
}

type AwardBlockRewardParams struct {
	Miner     address.Address
	Penalty   abi.TokenAmount // penalty for including bad messages in a block
	GasReward abi.TokenAmount // gas reward from all gas fees in a block
	WinCount  int64
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
	actorlog.L.Info("call reward actor AwardBlockReward")
	rt.ValidateImmediateCallerIs(builtin.SystemActorAddr)
	AssertMsg(rt.CurrentBalance().GreaterThanEqual(params.GasReward),
		"actor current balance %v insufficient to pay gas reward %v", rt.CurrentBalance(), params.GasReward)

	minerAddr, ok := rt.ResolveAddress(params.Miner)
	if !ok {
		rt.Abortf(exitcode.ErrIllegalState, "failed to resolve given owner address")
	}

	priorBalance := rt.CurrentBalance()

	penalty := abi.NewTokenAmount(0)
	var st State
	rt.State().Readonly(&st)

	blockReward := big.Mul(st.ThisEpochReward, big.NewInt(params.WinCount))
	blockReward = big.Div(blockReward, big.NewInt(builtin.ExpectedLeadersPerEpoch))

	totalReward := big.Add(blockReward, params.GasReward)

	// Cap the penalty at the total reward value.
	penalty = big.Min(params.Penalty, totalReward)

	// Reduce the payable reward by the penalty.
	rewardPayable := big.Sub(totalReward, penalty)

	AssertMsg(big.Add(rewardPayable, penalty).LessThanEqual(priorBalance),
		"reward payable %v + penalty %v exceeds balance %v", rewardPayable, penalty, priorBalance)

	_, code := rt.Send(minerAddr, builtin.MethodsMiner.AddLockedFund, &rewardPayable, rewardPayable)
	builtin.RequireSuccess(rt, code, "failed to send reward to miner: %s", minerAddr)

	// Burn the penalty amount.
	_, code = rt.Send(builtin.BurntFundsActorAddr, builtin.MethodSend, nil, penalty)
	builtin.RequireSuccess(rt, code, "failed to send penalty to burnt funds actor")

	return nil
}

func (a Actor) ThisEpochReward(rt vmr.Runtime, _ *adt.EmptyValue) *abi.TokenAmount {
	rt.ValidateImmediateCallerAcceptAny()

	var st State
	rt.State().Readonly(&st)
	return &st.ThisEpochReward
}

// Called at the end of each epoch by the power actor (in turn by its cron hook).
// This is only invoked for non-empty tipsets. The impact of this is that block rewards are paid out over
// a schedule defined by non-empty tipsets, not by elapsed time/epochs.
// This is not necessarily what we want, and may change.
func (a Actor) UpdateNetworkKPI(rt vmr.Runtime, currRealizedPower *abi.StoragePower) *adt.EmptyValue {
	//rt.ValidateImmediateCallerIs(builtin.StoragePowerActorAddr)
	actorlog.L.Info("reward actor call update network kpi")
	if currRealizedPower == nil {
		rt.Abortf(exitcode.ErrIllegalArgument, "arugment should not be nil")
	}

	var st State
	rt.State().Transaction(&st, func() interface{} {
		actorlog.L.Info("the reward actor state before update is:",zap.Any("state",st))
		// if there were null runs catch up the computation until
		// st.Epoch == rt.CurrEpoch()
		log.Println("the st epoch and rt currEpoch is:",st.Epoch,rt.CurrEpoch())
		for st.Epoch < rt.CurrEpoch() {
			// Update to next epoch to process null rounds
			st.updateToNextEpoch(*currRealizedPower)
		}

		st.updateToNextEpochWithReward(*currRealizedPower)
		actorlog.L.Info("the reward actor state after update is:",zap.Any("state",st))
		return nil
	})
	actorlog.L.Info("")
	return nil
}
