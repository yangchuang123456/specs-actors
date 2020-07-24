package miner

import (
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
)

// IP = IPBase(precommit time) + AdditionalIP(precommit time)
// IPBase(t) = InitialPledgeFactor * BR(t)
// AdditionalIP(t) = LockTarget(t)*PledgeShare(t)
// LockTarget = (LockTargetFactorNum / LockTargetFactorDenom) * FILCirculatingSupply(t)
// PledgeShare(t) = sectorQAPower / max(BaselinePower(t), NetworkQAPower(t))
// PARAM_FINISH
var InitialPledgeFactor = big.NewInt(20)
var LockTargetFactorNum = big.NewInt(3)
var LockTargetFactorDenom = big.NewInt(10)

// FF = (DeclaredFaultFactorNum / DeclaredFaultFactorDenom) * BR(t)
var DeclaredFaultFactorNum = big.NewInt(214)
var DeclaredFaultFactorDenom = big.NewInt(100)

// SP = (UndeclaredFaultFactor / DeclaredFaultFactorDenom) * BR(t)
var UndeclaredFaultFactorNum = big.NewInt(5)
var UndeclaredFaultFactorDenom = big.NewInt(1)

// This is the BR(t) value of the given sector for the current epoch.
// It is the expected reward this sector would pay out over a one day period.
// BR(t) = CurrEpochReward(t) * SectorQualityAdjustedPower * EpochsInDay / TotalNetworkQualityAdjustedPower(t)
func ExpectedDayRewardForPower(epochTargetReward abi.TokenAmount, networkQAPower abi.StoragePower, qaSectorPower abi.StoragePower) abi.TokenAmount {
	if networkQAPower.IsZero() {
		return epochTargetReward
	}
	expectedRewardForProvingPeriod := big.Mul(big.NewInt(builtin.EpochsInDay), epochTargetReward)
	return big.Div(big.Mul(qaSectorPower, expectedRewardForProvingPeriod), networkQAPower)
}

// This is the FF(t) penalty for a sector expected to be in the fault state either because the fault was declared or because
// it has been previously detected by the network.
// FF(t) = DeclaredFaultFactor * BR(t)
func PledgePenaltyForDeclaredFault(epochReward abi.TokenAmount, networkQAPower abi.StoragePower, qaSectorPower abi.StoragePower) abi.TokenAmount {
	return big.NewInt(0)
	return big.Div(
		big.Mul(DeclaredFaultFactorNum, ExpectedDayRewardForPower(epochReward, networkQAPower, qaSectorPower)),
		DeclaredFaultFactorDenom)
}

// This is the SP(t) penalty for a newly faulty sector that has not been declared.
// SP(t) = UndeclaredFaultFactor * BR(t)
func PledgePenaltyForUndeclaredFault(epochReward abi.TokenAmount, networkQAPower abi.StoragePower, qaSectorPower abi.StoragePower) abi.TokenAmount {
	return big.NewInt(0)
	return big.Div(
		big.Mul(UndeclaredFaultFactorNum, ExpectedDayRewardForPower(epochReward, networkQAPower, qaSectorPower)),
		UndeclaredFaultFactorDenom)
}

// Penalty to locked pledge collateral for the termination of a sector before scheduled expiry.
// SectorAge is the time between the sector's activation and termination.
func PledgePenaltyForTermination(initialPledge abi.TokenAmount, sectorAge abi.ChainEpoch, epochTargetReward abi.TokenAmount, networkQAPower, qaSectorPower abi.StoragePower) abi.TokenAmount {
	return big.NewInt(0)
	// max(SP(t), IP + BR(StartEpoch)*min(SectorAgeInDays, 180))
	// where BR(StartEpoch)=IP/InitialPledgeFactor
	// and sectorAgeInDays = sectorAge / EpochsInDay
	cappedSectorAge := big.NewInt(int64(minEpoch(sectorAge, 180*builtin.EpochsInDay)))
	return big.Max(
		PledgePenaltyForUndeclaredFault(epochTargetReward, networkQAPower, qaSectorPower),
		big.Add(
			initialPledge,
			big.Div(
				big.Mul(initialPledge, cappedSectorAge),
				big.Mul(InitialPledgeFactor, big.NewInt(builtin.EpochsInDay)))))
}

// Computes the pledge requirement for committing new quality-adjusted power to the network, given the current
// total power, total pledge commitment, epoch block reward, and circulating token supply.
// In plain language, the pledge requirement is a multiple of the block reward expected to be earned by the
// newly-committed power, holding the per-epoch block reward constant (though in reality it will change over time).
func InitialPledgeForPower(qaPower abi.StoragePower, networkQAPower, baselinePower abi.StoragePower, networkTotalPledge abi.TokenAmount, epochTargetReward abi.TokenAmount, networkCirculatingSupply abi.TokenAmount) abi.TokenAmount {
	//return big.NewInt(0)
	ipBase := big.Mul(InitialPledgeFactor, ExpectedDayRewardForPower(epochTargetReward, networkQAPower, qaPower))

	lockTargetNum := big.Mul(LockTargetFactorNum, networkCirculatingSupply)
	lockTargetDenom := LockTargetFactorDenom
	pledgeShareNum := qaPower
	pledgeShareDenom := big.Max(big.Max(networkQAPower, baselinePower), qaPower) // use qaPower in case others are 0
	additionalIPNum := big.Mul(lockTargetNum, pledgeShareNum)
	additionalIPDenom := big.Mul(lockTargetDenom, pledgeShareDenom)
	additionalIP := big.Div(additionalIPNum, additionalIPDenom)

	return big.Add(ipBase, additionalIP)
}
