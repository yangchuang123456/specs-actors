package reward_test

import (
	"context"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/reward"
	"github.com/filecoin-project/specs-actors/support/mock"
	tutil "github.com/filecoin-project/specs-actors/support/testing"
)

func TestExports(t *testing.T) {
	mock.CheckActorExports(t, reward.Actor{})
}

func TestConstructor(t *testing.T) {
	actor := rewardHarness{reward.Actor{}, t}

	t.Run("construct with 0 power", func(t *testing.T) {
		rt := mock.NewBuilder(context.Background(), builtin.RewardActorAddr).
			WithCaller(builtin.SystemActorAddr, builtin.SystemActorCodeID).
			Build(t)
		startRealizedPower := abi.NewStoragePower(0)
		actor.constructAndVerify(rt, &startRealizedPower)
		st := getState(rt)
		assert.Equal(t, abi.ChainEpoch(0), st.Epoch)
		assert.Equal(t, abi.NewStoragePower(0), st.CumsumRealized)
		assert.Equal(t, big.MustFromString("9152074749760199658"), st.ThisEpochReward)
	})
	t.Run("construct with some power", func(t *testing.T) {
		rt := mock.NewBuilder(context.Background(), builtin.RewardActorAddr).
			WithCaller(builtin.SystemActorAddr, builtin.SystemActorCodeID).
			Build(t)
		startRealizedPower := big.Lsh(abi.NewStoragePower(1), 39)
		actor.constructAndVerify(rt, &startRealizedPower)
		st := getState(rt)
		assert.Equal(t, abi.ChainEpoch(0), st.Epoch)
		assert.Equal(t, startRealizedPower, st.CumsumRealized)

		// Note this check is sensative to the value of startRealizedPower and the minting function
		// so it is somewhat brittle. Values of startRealizedPower below 1<<20 mint no coins
		assert.NotEqual(t, big.Zero(), st.ThisEpochReward)
		assert.Equal(t, big.MustFromString("50336408296765376121"), st.ThisEpochReward)
	})
}

func TestAwardBlockReward(t *testing.T) {
	actor := rewardHarness{reward.Actor{}, t}
	builder := mock.NewBuilder(context.Background(), builtin.RewardActorAddr).
		WithCaller(builtin.SystemActorAddr, builtin.SystemActorCodeID)

	t.Run("assertion failure when current balance is less than gas reward", func(t *testing.T) {
		rt := builder.Build(t)
		startRealizedPower := abi.NewStoragePower(0)
		actor.constructAndVerify(rt, &startRealizedPower)
		miner := tutil.NewIDAddr(t, 1000)

		gasreward := abi.NewTokenAmount(10)
		rt.SetBalance(abi.NewTokenAmount(0))

		rt.ExpectValidateCallerAddr(builtin.SystemActorAddr)
		rt.ExpectAssertionFailure("actor current balance 0 insufficient to pay gas reward 10", func() {
			rt.Call(actor.AwardBlockReward, &reward.AwardBlockRewardParams{
				Miner:     miner,
				Penalty:   big.Zero(),
				GasReward: gasreward,
				WinCount:  1,
			})
		})
		rt.Verify()
	})

	t.Run("pays out current balance when reward exceeds total balance", func(t *testing.T) {
		rt := builder.Build(t)
		startRealizedPower := abi.NewStoragePower(1)
		actor.constructAndVerify(rt, &startRealizedPower)
		miner := tutil.NewIDAddr(t, 1000)

		// Total reward is a huge number, upon writing ~1e18, so 300 should be way less
		smallReward := abi.NewTokenAmount(300)
		rt.SetBalance(smallReward)
		rt.ExpectValidateCallerAddr(builtin.SystemActorAddr)
		rt.ExpectSend(miner, builtin.MethodsMiner.AddLockedFund, &smallReward, smallReward, nil, 0)
		rt.Call(actor.AwardBlockReward, &reward.AwardBlockRewardParams{
			Miner:     miner,
			Penalty:   big.Zero(),
			GasReward: big.Zero(),
			WinCount:  1,
		})
		rt.Verify()
	})
}

type rewardHarness struct {
	reward.Actor
	t testing.TB
}

func (h *rewardHarness) constructAndVerify(rt *mock.Runtime, currRawPower *abi.StoragePower) {
	rt.ExpectValidateCallerAddr(builtin.SystemActorAddr)
	ret := rt.Call(h.Constructor, currRawPower)
	assert.Nil(h.t, ret)
	rt.Verify()

}

func getState(rt *mock.Runtime) *reward.State {
	var st reward.State
	rt.GetState(&st)
	return &st
}


func Test_reward(t *testing.T) {
	actor := rewardHarness{reward.Actor{}, t}
	rt := mock.NewBuilder(context.Background(), builtin.RewardActorAddr).
		WithCaller(builtin.SystemActorAddr, builtin.SystemActorCodeID).
		Build(t)
	startRealizedPower := abi.NewStoragePower(0)
	actor.constructAndVerify(rt, &startRealizedPower)
	st := getState(rt)
	st.Print()

	rt.SetCaller(builtin.StoragePowerActorAddr, builtin.StoragePowerActorCodeID)
	rt.ExpectValidateCallerAny()
	rt.ExpectValidateCallerAddr(builtin.StoragePowerActorAddr)
	currRealizedPower := reward.BaselinePowerNextEpoch(reward.BaselineInitialValue)
	log.Println("the currRealizedPower is:",currRealizedPower)
	//	epochAddPower = big.NewInt(0)
	for i:=1;i<=100;i++{

		rt.Call(actor.UpdateNetworkKPI,&currRealizedPower)
		//currRealizedPower=big.Add(currRealizedPower,epochAddPower)
		st.Print()
/*		if i>=50{
			currRealizedPower = big.NewInt(0)
		}*/
		currRealizedPower = reward.BaselinePowerNextEpoch(currRealizedPower)
		log.Println("")
	}
}