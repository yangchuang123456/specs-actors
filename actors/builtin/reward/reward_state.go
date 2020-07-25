package reward

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	abi "github.com/filecoin-project/specs-actors/actors/abi"
	big "github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/util"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/filecoin-project/specs-actors/support/ipld"
	"github.com/filecoin-project/specs-actors/tools/dlog/actorlog"
	"github.com/ipfs/go-cid"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/xerrors"
	"io/ioutil"
	"log"
	big2 "math/big"
	"runtime"
)

// A quantity of space * time (in byte-epochs) representing power committed to the network for some duration.
type Spacetime = big.Int

type OneEpochRecord struct {
	Epoch                     abi.ChainEpoch
	BaseLineReward            abi.TokenAmount
	SimpleReward              abi.TokenAmount
	EpochReward               abi.TokenAmount
	InvestorAndProtoRelease   abi.TokenAmount
	NetworkCirculatingSupply  abi.TokenAmount
	NetWorkTotalReward        *abi.TokenAmount
	NetWorkTotalDeposit       *abi.TokenAmount
	NetWorkRewardLockFunds    *abi.TokenAmount
	NetWorkDepositLockFunds   *abi.TokenAmount
	NetWorkRewardUnLockFunds  *abi.TokenAmount
	NetWorkDepositUnLockFunds *abi.TokenAmount

	CumSumRealized         abi.StoragePower
	CumSumBaselinePower    abi.StoragePower
	EffectiveNetworkTime   abi.ChainEpoch
	TotalNetworkPower      abi.StoragePower
	BaseLinePower          abi.StoragePower
	EffectiveBaselinePower abi.StoragePower

	IPFSMainAddPower       abi.StoragePower
	IPFSMainTotalPower     abi.StoragePower
	IPFSMainExpectedReward abi.TokenAmount
	IPFSMainTotalReward    *abi.TokenAmount

	IPFSMainIP                 abi.TokenAmount
	IPFSMainTotalDeposit       *abi.TokenAmount
	IPFSMainRewardLockFunds    *abi.TokenAmount
	IPFSMainDepositLockFunds   *abi.TokenAmount
	IPFSMainRewardUnLockFunds  *abi.TokenAmount
	IPFSMainDepositUnLockFunds *abi.TokenAmount
}

type OneEpochRecordPrint struct {
	Epoch                     abi.ChainEpoch
	BaseLineReward            float64
	SimpleReward              float64
	EpochReward               float64
	InvestorAndProtoRelease   float64
	NetworkCirculatingSupply  float64
	NetWorkTotalReward        float64
	NetWorkTotalDeposit       float64
	NetWorkRewardLockFunds    float64
	NetWorkDepositLockFunds   float64
	NetWorkRewardUnLockFunds  float64
	NetWorkDepositUnLockFunds float64

	CumSumRealized         abi.StoragePower
	CumSumBaselinePower    abi.StoragePower
	EffectiveNetworkTime   abi.ChainEpoch
	TotalNetworkPower      abi.StoragePower
	BaseLinePower          abi.StoragePower
	EffectiveBaselinePower abi.StoragePower

	IPFSMainAddPower       abi.StoragePower
	IPFSMainTotalPower     abi.StoragePower
	IPFSMainExpectedReward float64
	IPFSMainTotalReward    float64

	IPFSMainIP                 float64
	IPFSMainTotalDeposit       float64
	IPFSMainRewardLockFunds    float64
	IPFSMainDepositLockFunds   float64
	IPFSMainRewardUnLockFunds  float64
	IPFSMainDepositUnLockFunds float64
}

//for test
type Record struct {
	RecordStartEpoch       abi.ChainEpoch
	RecordStep             abi.ChainEpoch
	RecordNumber           int64
	CurrentNeedRecordEpoch abi.ChainEpoch
	CurrentEpoch           abi.ChainEpoch
	Store                  adt.Store

	CirculateEnoughEpoch abi.ChainEpoch

	IPFSMainRewardLockFund  *cid.Cid // Array, AMT[ChainEpoch]TokenAmount
	IPFSMainDepositLockFund *cid.Cid

	NetWorkRewardLockFund  *cid.Cid
	NetWorkDepositLockFund *cid.Cid

	LastEpochRecord    OneEpochRecord
	CurrentEpochRecord OneEpochRecord
	NeedRecordEpoch    []OneEpochRecord
}

type PrintRecord struct {
	CirculateEnoughEpoch       abi.ChainEpoch
	Epoch                      []abi.ChainEpoch
	BaseLineReward             []float64
	SimpleReward               []float64
	EpochReward                []float64
	InvestorAndProtoRelease    []float64
	NetworkCirculatingSupply   []float64
	NetWorkTotalReward         []float64
	NetWorkTotalDeposit        []float64
	NetWorkRewardLockFunds     []float64
	NetWorkDepositLockFunds    []float64
	NetWorkRewardUnLockFunds   []float64
	NetWorkDepositUnLockFunds  []float64
	CumSumRealized             []abi.StoragePower
	CumSumBaselinePower        []abi.StoragePower
	EffectiveNetworkTime       []abi.ChainEpoch
	TotalNetworkPower          []abi.StoragePower
	BaseLinePower              []abi.StoragePower
	EffectiveBaselinePower     []abi.StoragePower
	IPFSMainAddPower           []abi.StoragePower
	IPFSMainTotalPower         []abi.StoragePower
	IPFSMainExpectedReward     []float64
	IPFSMainTotalReward        []float64
	IPFSMainIP                 []float64
	IPFSMainTotalDeposit       []float64
	IPFSMainRewardLockFunds    []float64
	IPFSMainDepositLockFunds   []float64
	IPFSMainRewardUnLockFunds  []float64
	IPFSMainDepositUnLockFunds []float64
}

func NewPrintRecord(epoch int64) *PrintRecord {
	return &PrintRecord{
		Epoch:                     make([]abi.ChainEpoch, epoch),
		BaseLineReward:            make([]float64, epoch),
		SimpleReward:              make([]float64, epoch),
		EpochReward:               make([]float64, epoch),
		InvestorAndProtoRelease:   make([]float64, epoch),
		NetworkCirculatingSupply:  make([]float64, epoch),
		NetWorkTotalReward:        make([]float64, epoch),
		NetWorkTotalDeposit:       make([]float64, epoch),
		NetWorkRewardLockFunds:    make([]float64, epoch),
		NetWorkDepositLockFunds:   make([]float64, epoch),
		NetWorkRewardUnLockFunds:  make([]float64, epoch),
		NetWorkDepositUnLockFunds: make([]float64, epoch),


		CumSumRealized:             make([]abi.StoragePower, epoch),
		CumSumBaselinePower:        make([]abi.StoragePower, epoch),
		EffectiveNetworkTime:       make([]abi.ChainEpoch, epoch),
		TotalNetworkPower:          make([]abi.StoragePower, epoch),
		BaseLinePower:              make([]abi.StoragePower, epoch),
		EffectiveBaselinePower:     make([]abi.StoragePower, epoch),
		IPFSMainTotalPower:         make([]abi.StoragePower, epoch),
		IPFSMainAddPower:           make([]abi.StoragePower, epoch),
		IPFSMainExpectedReward:     make([]float64, epoch),
		IPFSMainIP:                 make([]float64, epoch),
		IPFSMainTotalReward:        make([]float64, epoch),
		IPFSMainTotalDeposit:       make([]float64, epoch),
		IPFSMainRewardLockFunds:    make([]float64, epoch),
		IPFSMainDepositLockFunds:   make([]float64, epoch),
		IPFSMainRewardUnLockFunds:  make([]float64, epoch),
		IPFSMainDepositUnLockFunds: make([]float64, epoch),
	}
}

var CirculateNotEnough = abi.ChainEpoch(-2) //-2标识未到达

func NewRewardRecord(recordStartEpoch, stepEpoch abi.ChainEpoch, RecordNumber int64) *Record {
	store := ipld.NewADTStore(context.Background())
	iPFSMainRewardLockFund, err := adt.MakeEmptyArray(store).Root()
	if err != nil {
		panic("NewRewardRecord MakeEmptyArray iPFSMainRewardLockFund error")
	}
	iPFSMainDepositLockFund, err := adt.MakeEmptyArray(store).Root()
	if err != nil {
		panic("NewRewardRecord MakeEmptyArray iPFSMainDepositLockFund error")
	}
	netWorkRewardLockFund, err := adt.MakeEmptyArray(store).Root()
	if err != nil {
		panic("NewRewardRecord MakeEmptyArray netWorkRewardLockFund error")
	}
	netWorkDepositLockFund, err := adt.MakeEmptyArray(store).Root()
	if err != nil {
		panic("NewRewardRecord MakeEmptyArray netWorkDepositLockFund error")
	}

	return &Record{
		RecordStartEpoch:        recordStartEpoch,
		RecordStep:              stepEpoch,
		RecordNumber:            RecordNumber,
		CurrentNeedRecordEpoch:  recordStartEpoch,
		CirculateEnoughEpoch:    CirculateNotEnough,
		Store:                   store,
		CurrentEpoch:            abi.ChainEpoch(0),
		IPFSMainRewardLockFund:  &iPFSMainRewardLockFund,
		IPFSMainDepositLockFund: &iPFSMainDepositLockFund,
		NetWorkRewardLockFund:   &netWorkRewardLockFund,
		NetWorkDepositLockFund:  &netWorkDepositLockFund,
		LastEpochRecord:         OneEpochRecord{},
		CurrentEpochRecord:      OneEpochRecord{},
		NeedRecordEpoch:         make([]OneEpochRecord, 0),
	}
}

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

	*Record
}

func ConstructState(currRealizedPower abi.StoragePower) *State {
	actorlog.L.Info("call reward state ConstructState")
	st := &State{
		CumsumBaseline:         big.Zero(),
		CumsumRealized:         big.Zero(),
		EffectiveNetworkTime:   0,
		EffectiveBaselinePower: BaselineInitialValue,

		ThisEpochReward:        big.Zero(),
		ThisEpochBaselinePower: InitBaselinePower(),
		Epoch:                  -1,
	}

	st.updateToNextEpochWithReward(currRealizedPower)
	st.Print()

	return st
}

// Takes in current realized power and updates internal state
// Used for update of internal state during null rounds
func (st *State) updateToNextEpoch(currRealizedPower abi.StoragePower) {
	//	actorlog.L.Info("updateToNextEpoch start the state is:",zap.Any("stAddr",fmt.Sprintf("%p",st)))
	//st.Print()
	st.Epoch++
	st.ThisEpochBaselinePower = BaselinePowerFromPrev(st.ThisEpochBaselinePower)
	cappedRealizedPower := big.Min(st.ThisEpochBaselinePower, currRealizedPower)
	st.CumsumRealized = big.Add(st.CumsumRealized, cappedRealizedPower)
	//	actorlog.L.Info("updateToNextEpoch execute is:333",zap.Any("st.EffectiveBaselinePower",st.EffectiveBaselinePower))
	//st.Print()
	for st.CumsumRealized.GreaterThan(st.CumsumBaseline) {
		actorlog.L.Info("st.CumsumRealized.GreaterThan(st.CumsumBaseline)", zap.Any("CumsumRealized", st.CumsumRealized), zap.Any("CumsumBaseline", st.CumsumBaseline), zap.Any("epoch", st.Epoch))
		st.EffectiveNetworkTime++
		st.EffectiveBaselinePower = BaselinePowerFromPrev(st.EffectiveBaselinePower)
		st.CumsumBaseline = big.Add(st.CumsumBaseline, st.EffectiveBaselinePower)
	}
	//actorlog.L.Info("updateToNextEpoch end the state is:")
	//st.Print()
}

// Takes in a current realized power for a reward epoch and computes
// and updates reward state to track reward for the next epoch
func (st *State) updateToNextEpochWithReward(currRealizedPower abi.StoragePower) {
	prevRewardTheta := computeRTheta(st.EffectiveNetworkTime, st.EffectiveBaselinePower, st.CumsumRealized, st.CumsumBaseline)
	st.updateToNextEpoch(currRealizedPower)
	currRewardTheta := computeRTheta(st.EffectiveNetworkTime, st.EffectiveBaselinePower, st.CumsumRealized, st.CumsumBaseline)
	log.Println("the prevRewardTheta and currRewardTheta is:", q128ToF(prevRewardTheta), q128ToF(currRewardTheta))
	st.ThisEpochReward = computeReward(st.Epoch, prevRewardTheta, currRewardTheta)
}

func (st *State) updateToNextEpochWithRewardForTest(totalAdd, ipfsMainAdd, currRealizedPower abi.StoragePower) {
	//log.Println("the current epoch is:", st.Epoch)
	//st.PrintOneEpoch(st.Epoch)
	prevRewardTheta := computeRTheta(st.EffectiveNetworkTime, st.EffectiveBaselinePower, st.CumsumRealized, st.CumsumBaseline)
	st.updateToNextEpoch(currRealizedPower)
	currRewardTheta := computeRTheta(st.EffectiveNetworkTime, st.EffectiveBaselinePower, st.CumsumRealized, st.CumsumBaseline)
	//log.Println("the prevRewardTheta and currRewardTheta is:", q128ToF(prevRewardTheta), q128ToF(currRewardTheta))
	totalReward, simpleReward, baseLineReward := computeRewardForTest(st.Epoch, prevRewardTheta, currRewardTheta)
	st.ThisEpochReward = totalReward

	st.LastEpochRecord = st.CurrentEpochRecord

	st.Record.CurrentEpoch = st.Epoch
	st.Record.CurrentEpochRecord.Epoch = st.Epoch
	st.Record.CurrentEpochRecord.EffectiveNetworkTime = st.EffectiveNetworkTime
	st.Record.CurrentEpochRecord.BaseLinePower = st.ThisEpochBaselinePower
	st.Record.CurrentEpochRecord.CumSumBaselinePower = st.CumsumBaseline
	st.Record.CurrentEpochRecord.CumSumRealized = st.CumsumRealized
	st.Record.CurrentEpochRecord.TotalNetworkPower = currRealizedPower
	st.Record.CurrentEpochRecord.EffectiveBaselinePower = st.EffectiveBaselinePower

	st.Record.CurrentEpochRecord.EpochReward = totalReward
	st.Record.CurrentEpochRecord.SimpleReward = simpleReward
	st.Record.CurrentEpochRecord.BaseLineReward = baseLineReward

	if st.Epoch == 0 {
		st.Record.CurrentEpochRecord.InvestorAndProtoRelease = big.NewInt(0)
		st.Record.CurrentEpochRecord.NetworkCirculatingSupply = big.NewInt(0)
		st.Record.CurrentEpochRecord.IPFSMainAddPower = big.NewInt(0)
		st.Record.CurrentEpochRecord.IPFSMainTotalPower = big.NewInt(0)
		st.Record.CurrentEpochRecord.IPFSMainExpectedReward = big.NewInt(0)
		st.Record.CurrentEpochRecord.IPFSMainIP = big.NewInt(0)
	}

	st.Record.CurrentEpochRecord.NetWorkTotalReward = &abi.TokenAmount{big2.NewInt(0)}
	st.Record.CurrentEpochRecord.NetWorkTotalDeposit = &abi.TokenAmount{big2.NewInt(0)}
	st.Record.CurrentEpochRecord.NetWorkRewardLockFunds = &abi.TokenAmount{big2.NewInt(0)}
	st.Record.CurrentEpochRecord.NetWorkDepositLockFunds = &abi.TokenAmount{big2.NewInt(0)}
	st.Record.CurrentEpochRecord.NetWorkRewardUnLockFunds = &abi.TokenAmount{big2.NewInt(0)}
	st.Record.CurrentEpochRecord.NetWorkDepositUnLockFunds = &abi.TokenAmount{big2.NewInt(0)}
	st.Record.CurrentEpochRecord.IPFSMainTotalReward = &abi.TokenAmount{big2.NewInt(0)}
	st.Record.CurrentEpochRecord.IPFSMainTotalDeposit = &abi.TokenAmount{big2.NewInt(0)}
	st.Record.CurrentEpochRecord.IPFSMainRewardLockFunds = &abi.TokenAmount{big2.NewInt(0)}
	st.Record.CurrentEpochRecord.IPFSMainDepositLockFunds = &abi.TokenAmount{big2.NewInt(0)}
	st.Record.CurrentEpochRecord.IPFSMainRewardUnLockFunds = &abi.TokenAmount{big2.NewInt(0)}
	st.Record.CurrentEpochRecord.IPFSMainDepositUnLockFunds = &abi.TokenAmount{big2.NewInt(0)}

	//正式计算抵押时，使用的power为qaPower,此处假设全网皆为cc扇区，不受deal和duration影响，直接使用rawPower计算
	if st.Epoch > 0 {
		qaPower := totalAdd
		networkQAPower := st.Record.CurrentEpochRecord.TotalNetworkPower
		baselinePower := st.Record.CurrentEpochRecord.BaseLinePower
		networkTotalPledge := *st.Record.LastEpochRecord.NetWorkTotalDeposit
		epochTargetReward := st.ThisEpochReward
		networkCirculatingSupply := st.LastEpochRecord.NetworkCirculatingSupply
		currentNetWorkDeposit := InitialPledgeForPower(qaPower, networkQAPower, baselinePower, networkTotalPledge, epochTargetReward, networkCirculatingSupply)

		st.Record.UpdateLockAndUnlockFunds(totalReward, &RewardVestingSpec, NetworkRewardLock)
		st.Record.UpdateLockAndUnlockFunds(currentNetWorkDeposit, &PledgeVestingSpec, NetWorkDeposit)
		st.Record.UpdateCirculatingSupply()
	}
	st.paddingIPFSMain(ipfsMainAdd)

	if st.CurrentEpoch == st.CurrentNeedRecordEpoch {
		st.NeedRecordEpoch = append(st.NeedRecordEpoch, st.CurrentEpochRecord)
		st.CurrentNeedRecordEpoch += st.RecordStep
	}

	if st.CurrentEpoch%(60/25) == 0{
		runtime.GC()
	}
}

type LockType int

const (
	NetworkRewardLock LockType = iota
	NetWorkDeposit
	IPFSMainRewardLock
	IPFSMainDeposit
)

// Specification for a linear vesting schedule.
type VestSpec struct {
	InitialDelay abi.ChainEpoch // Delay before any amount starts vesting.
	VestPeriod   abi.ChainEpoch // Period over which the total should vest, after the initial delay.
	StepDuration abi.ChainEpoch // Duration between successive incremental vests (independent of vesting period).
	Quantization abi.ChainEpoch // Maximum precision of vesting table (limits cardinality of table).
}

var PledgeVestingSpec = VestSpec{
	InitialDelay: abi.ChainEpoch(180 * builtin.EpochsInDay), // PARAM_FINISH
	VestPeriod:   abi.ChainEpoch(180 * builtin.EpochsInDay), // PARAM_FINISH
	StepDuration: abi.ChainEpoch(1 * builtin.EpochsInDay),   // PARAM_FINISH
	Quantization: 12 * builtin.EpochsInHour,                 // PARAM_FINISH
}

var RewardVestingSpec = VestSpec{
	InitialDelay: abi.ChainEpoch(20 * builtin.EpochsInDay),  // PARAM_FINISH
	VestPeriod:   abi.ChainEpoch(180 * builtin.EpochsInDay), // PARAM_FINISH
	StepDuration: abi.ChainEpoch(1 * builtin.EpochsInDay),   // PARAM_FINISH
	Quantization: 12 * builtin.EpochsInHour,                 // PARAM_FINISH
}

func quantizeUp(e abi.ChainEpoch, unit abi.ChainEpoch, offsetSeed abi.ChainEpoch) abi.ChainEpoch {
	offset := offsetSeed % unit

	remainder := (e - offset) % unit
	quotient := (e - offset) / unit
	// Don't round if epoch falls on a quantization epoch
	if remainder == 0 {
		return unit*quotient + offset
	}
	// Negative truncating division rounds up
	if e-offset < 0 {
		return unit*quotient + offset
	}
	return unit*(quotient+1) + offset

}

func AddLockedFunds(store adt.Store, root *cid.Cid, currEpoch abi.ChainEpoch, vestingSum abi.TokenAmount, spec *VestSpec) error {
	util.AssertMsg(vestingSum.GreaterThanEqual(big.Zero()), "negative vesting sum %s", vestingSum)
	vestingFunds, err := adt.AsArray(store, *root)
	if err != nil {
		return err
	}

	// Quantization is aligned with when regular cron will be invoked, in the last epoch of deadlines.
	vestBegin := currEpoch + spec.InitialDelay // Nothing unlocks here, this is just the start of the clock.
	vestPeriod := big.NewInt(int64(spec.VestPeriod))
	vestedSoFar := big.Zero()
	for e := vestBegin + spec.StepDuration; vestedSoFar.LessThan(vestingSum); e += spec.StepDuration {
		vestEpoch := quantizeUp(e, spec.Quantization, 0)
		elapsed := vestEpoch - vestBegin

		targetVest := big.Zero() //nolint:ineffassign
		if elapsed < spec.VestPeriod {
			// Linear vesting, PARAM_FINISH
			targetVest = big.Div(big.Mul(vestingSum, big.NewInt(int64(elapsed))), vestPeriod)
		} else {
			targetVest = vestingSum
		}

		vestThisTime := big.Sub(targetVest, vestedSoFar)
		vestedSoFar = targetVest

		// Load existing entry, else set a new one
		key := uint64(vestEpoch)
		lockedFundEntry := big.Zero()
		_, err = vestingFunds.Get(key, &lockedFundEntry)
		if err != nil {
			return err
		}

		lockedFundEntry = big.Add(lockedFundEntry, vestThisTime)
		err = vestingFunds.Set(key, &lockedFundEntry)
		if err != nil {
			return err
		}
	}

	*root, err = vestingFunds.Root()
	if err != nil {
		return err
	}
	return nil
}

func UnlockVestedFunds(store adt.Store, root *cid.Cid, currEpoch abi.ChainEpoch) (abi.TokenAmount, error) {
	vestingFunds, err := adt.AsArray(store, *root)
	if err != nil {
		return big.Zero(), err
	}

	amountUnlocked := abi.NewTokenAmount(0)
	lockedEntry := abi.NewTokenAmount(0)
	var toDelete []uint64
	var finished = fmt.Errorf("finished")

	// Iterate vestingFunds  in order of release.
	err = vestingFunds.ForEach(&lockedEntry, func(k int64) error {
		if k < int64(currEpoch) {
			amountUnlocked = big.Add(amountUnlocked, lockedEntry)
			toDelete = append(toDelete, uint64(k))
		} else {
			return finished // stop iterating
		}
		return nil
	})

	if err != nil && err != finished {
		return big.Zero(), err
	}

	err = vestingFunds.BatchDelete(toDelete)
	if err != nil {
		return big.Zero(), errors.Wrapf(err, "failed to delete locked fund during vest: %v", err)
	}

	*root, err = vestingFunds.Root()
	if err != nil {
		return big.Zero(), err
	}

	return amountUnlocked, nil
}

func (r *Record) UpdateCirculatingSupply() {
	//更新投资人和协议实验室
	addValue := big.NewInt(0)
	if r.CurrentEpoch > 0 && r.CurrentEpoch <= 183*builtin.EpochsInDay {
		addValue = big.Div(big.Mul(big.NewInt(6938822776), big.NewInt(1e14)), big.NewInt(builtin.EpochsInDay))
		//addValue = big.Mul(big.NewInt(6938822776), big.NewInt(1e14))
	} else if r.CurrentEpoch > 183*builtin.EpochsInDay && r.CurrentEpoch <= 365*builtin.EpochsInDay {
		//addValue = big.Div(big.Mul(big.NewInt(4745936072), big.NewInt(1e14)),big.NewInt(builtin.EpochsInDay))
		addValue = big.Mul(big.NewInt(6938822776), big.NewInt(1e14))
	}

	r.CurrentEpochRecord.InvestorAndProtoRelease = big.Add(r.LastEpochRecord.InvestorAndProtoRelease, addValue)

	totalMoney := big.Add(r.CurrentEpochRecord.InvestorAndProtoRelease, *r.CurrentEpochRecord.NetWorkTotalReward)
	totalLock := big.Add(*r.CurrentEpochRecord.NetWorkDepositLockFunds, *r.CurrentEpochRecord.NetWorkRewardLockFunds)

	sub := big.Sub(totalMoney, totalLock)
	if big.Cmp(sub, big.NewInt(0)) == -1 {
		sub = big.NewInt(0)
	} else if r.CirculateEnoughEpoch == CirculateNotEnough {
		r.CirculateEnoughEpoch = r.CurrentEpoch
	}
	r.CurrentEpochRecord.NetworkCirculatingSupply = sub
}

func (r *Record) UpdateLockAndUnlockFunds(vestingSum abi.TokenAmount, spec *VestSpec, lockType LockType) error {
	var root *cid.Cid
	var totalMoney, Locked, Unlocked, lastUnlocked, lastTotalMoney *abi.TokenAmount
	switch lockType {
	case NetworkRewardLock:
		root = r.NetWorkRewardLockFund
		lastTotalMoney = r.LastEpochRecord.NetWorkTotalReward
		lastUnlocked = r.LastEpochRecord.NetWorkRewardUnLockFunds
		totalMoney = r.CurrentEpochRecord.NetWorkTotalReward
		Locked = r.CurrentEpochRecord.NetWorkRewardLockFunds
		Unlocked = r.CurrentEpochRecord.NetWorkRewardUnLockFunds
	case NetWorkDeposit:
		root = r.NetWorkDepositLockFund
		lastTotalMoney = r.LastEpochRecord.NetWorkTotalDeposit
		lastUnlocked = r.LastEpochRecord.NetWorkDepositUnLockFunds
		totalMoney = r.CurrentEpochRecord.NetWorkTotalDeposit
		Locked = r.CurrentEpochRecord.NetWorkDepositLockFunds
		Unlocked = r.CurrentEpochRecord.NetWorkDepositUnLockFunds
	case IPFSMainRewardLock:
		root = r.IPFSMainRewardLockFund
		lastTotalMoney = r.LastEpochRecord.IPFSMainTotalReward
		lastUnlocked = r.LastEpochRecord.IPFSMainRewardUnLockFunds
		totalMoney = r.CurrentEpochRecord.IPFSMainTotalReward
		Locked = r.CurrentEpochRecord.IPFSMainRewardLockFunds
		Unlocked = r.CurrentEpochRecord.IPFSMainRewardUnLockFunds
	case IPFSMainDeposit:
		root = r.IPFSMainDepositLockFund
		lastTotalMoney = r.LastEpochRecord.IPFSMainTotalDeposit
		lastUnlocked = r.LastEpochRecord.IPFSMainDepositUnLockFunds
		totalMoney = r.CurrentEpochRecord.IPFSMainTotalDeposit
		Locked = r.CurrentEpochRecord.IPFSMainDepositLockFunds
		Unlocked = r.CurrentEpochRecord.IPFSMainDepositUnLockFunds
	default:
		return errors.New("lockType error")
	}

	//log.Println("the lockType,currentEpoch,vestingSum and lastTotalMoney is",lockType,r.CurrentEpoch,vestingSum,*lastTotalMoney)

	err := AddLockedFunds(r.Store, root, r.CurrentEpoch, vestingSum, spec)
	if err != nil {
		return err
	}
	//log.Println("last totalMoney is:",*lastTotalMoney)
	*totalMoney = big.Add(*lastTotalMoney, vestingSum)
	//log.Println("current totalMoney is:",*totalMoney)

	tmp, err := UnlockVestedFunds(r.Store, root, r.CurrentEpoch)
	if err != nil {
		return err
	}
	*Unlocked = big.Add(*lastUnlocked, tmp)
	*Locked = big.Sub(*totalMoney, *Unlocked)

	//log.Println("the state total money is:",*r.CurrentEpochRecord.NetWorkTotalReward)
	return nil
}

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

// IP = IPBase(precommit time) + AdditionalIP(precommit time)
// IPBase(t) = InitialPledgeFactor * BR(t)
// AdditionalIP(t) = LockTarget(t)*PledgeShare(t)
// LockTarget = (LockTargetFactorNum / LockTargetFactorDenom) * FILCirculatingSupply(t)
// PledgeShare(t) = sectorQAPower / max(BaselinePower(t), NetworkQAPower(t))
// PARAM_FINISH
var InitialPledgeFactor = big.NewInt(20)
var LockTargetFactorNum = big.NewInt(3)
var LockTargetFactorDenom = big.NewInt(10)

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
	//log.Println("the qaPower and networkQAPower and baselinePower is:",qaPower,networkQAPower,baselinePower)
	//log.Println("the ipBase and additionalIP is:",ipBase,additionalIP)
	return big.Add(ipBase, additionalIP)
}

func (st *State) paddingIPFSMain(IPFSMainEpochAddPower abi.StoragePower) {
	st.Record.CurrentEpochRecord.IPFSMainAddPower = IPFSMainEpochAddPower
	if st.Epoch <= 0 {
		return
	}

	//IPFSMain当前epoch的预期奖励＝epoch-1的total power占比乘以当前epoch的奖励
	//IPFSMain当前epoch的总奖励=前epoch-1个epoch的预期奖励之和
	st.Record.CurrentEpochRecord.IPFSMainTotalPower = big.Add(st.Record.LastEpochRecord.IPFSMainTotalPower, IPFSMainEpochAddPower)
	expectedReward := big.Div(big.Mul(st.ThisEpochReward, st.Record.LastEpochRecord.IPFSMainTotalPower), st.Record.LastEpochRecord.TotalNetworkPower)
	st.Record.CurrentEpochRecord.IPFSMainExpectedReward = expectedReward
	st.Record.UpdateLockAndUnlockFunds(expectedReward, &RewardVestingSpec, IPFSMainRewardLock)

	//将全网流通量设为0，计算所得即为IPBase。
	//正式计算抵押时，使用的power为qaPower,此处假设全网皆为cc扇区，不受deal和duration影响，直接使用rawPower计算
	qaPower := IPFSMainEpochAddPower
	networkQAPower := st.Record.CurrentEpochRecord.TotalNetworkPower
	baselinePower := st.Record.CurrentEpochRecord.BaseLinePower
	networkTotalPledge := *st.Record.LastEpochRecord.NetWorkTotalDeposit
	epochTargetReward := st.ThisEpochReward
	networkCirculatingSupply := st.Record.LastEpochRecord.NetworkCirculatingSupply
	st.Record.CurrentEpochRecord.IPFSMainIP = InitialPledgeForPower(qaPower, networkQAPower, baselinePower, networkTotalPledge, epochTargetReward, networkCirculatingSupply)

	st.Record.UpdateLockAndUnlockFunds(st.Record.CurrentEpochRecord.IPFSMainIP, &PledgeVestingSpec, IPFSMainDeposit)
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

func (r *Record) SaveToFile() error {
	tmpRecordPoint := r.GetRecordPoint()
	b, err := json.MarshalIndent(tmpRecordPoint, "", "  ")
	if err != nil {
		return xerrors.Errorf("marshaling Record file: %w", err)
	}
	/*	b, err := json.Marshal(tmpRecordPoint)
		if err != nil {
			return xerrors.Errorf("marshaling Record file: %w", err)
		}*/
	if err := ioutil.WriteFile("record.json", b, 0644); err != nil {
		return xerrors.Errorf("persisting storage config (%s): %w", "record.json", err)
	}
	return nil
}

func (r *Record) GetCurrentPrintEpochData() OneEpochRecordPrint {
	tmp := OneEpochRecordPrint{
		Epoch:                     r.CurrentEpochRecord.Epoch,
		BaseLineReward:            ToFile(r.CurrentEpochRecord.BaseLineReward),
		SimpleReward:              ToFile(r.CurrentEpochRecord.SimpleReward),
		EpochReward:               ToFile(r.CurrentEpochRecord.EpochReward),
		InvestorAndProtoRelease:   ToFile(r.CurrentEpochRecord.InvestorAndProtoRelease),
		NetworkCirculatingSupply:  ToFile(r.CurrentEpochRecord.NetworkCirculatingSupply),
		NetWorkTotalReward:        ToFile(*r.CurrentEpochRecord.NetWorkTotalReward),
		NetWorkTotalDeposit:       ToFile(*r.CurrentEpochRecord.NetWorkTotalDeposit),
		NetWorkRewardLockFunds:    ToFile(*r.CurrentEpochRecord.NetWorkRewardLockFunds),
		NetWorkDepositLockFunds:   ToFile(*r.CurrentEpochRecord.NetWorkDepositLockFunds),
		NetWorkRewardUnLockFunds:  ToFile(*r.CurrentEpochRecord.NetWorkRewardUnLockFunds),
		NetWorkDepositUnLockFunds: ToFile(*r.CurrentEpochRecord.NetWorkDepositUnLockFunds),

		CumSumRealized:         r.CurrentEpochRecord.CumSumRealized,
		CumSumBaselinePower:    r.CurrentEpochRecord.CumSumBaselinePower,
		EffectiveNetworkTime:   r.CurrentEpochRecord.EffectiveNetworkTime,
		TotalNetworkPower:      r.CurrentEpochRecord.TotalNetworkPower,
		BaseLinePower:          r.CurrentEpochRecord.BaseLinePower,
		EffectiveBaselinePower: r.CurrentEpochRecord.EffectiveBaselinePower,

		IPFSMainAddPower:       r.CurrentEpochRecord.IPFSMainAddPower,
		IPFSMainTotalPower:     r.CurrentEpochRecord.IPFSMainTotalPower,
		IPFSMainExpectedReward: ToFile(r.CurrentEpochRecord.IPFSMainExpectedReward),
		IPFSMainTotalReward:    ToFile(*r.CurrentEpochRecord.IPFSMainTotalReward),

		IPFSMainIP:                 ToFile(r.CurrentEpochRecord.IPFSMainIP),
		IPFSMainTotalDeposit:       ToFile(*r.CurrentEpochRecord.IPFSMainTotalDeposit),
		IPFSMainRewardLockFunds:    ToFile(*r.CurrentEpochRecord.IPFSMainRewardLockFunds),
		IPFSMainDepositLockFunds:   ToFile(*r.CurrentEpochRecord.IPFSMainDepositLockFunds),
		IPFSMainRewardUnLockFunds:  ToFile(*r.CurrentEpochRecord.IPFSMainRewardUnLockFunds),
		IPFSMainDepositUnLockFunds: ToFile(*r.CurrentEpochRecord.IPFSMainDepositUnLockFunds),
	}
	return tmp
}

func (r *Record) PrintCurrentEpoch() error {
	log.Println("one Epoch economy model record")
	tmp := r.GetCurrentPrintEpochData()
	byte, err := json.Marshal(&tmp)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	json.Indent(&out, byte, "", "\t")
	log.Println(out.String())
	return nil
}

func (r *Record) GetRecordPoint() *PrintRecord {
	tmpRecord := NewPrintRecord(r.RecordNumber)
	i := 0
	tmpRecord.CirculateEnoughEpoch = r.CirculateEnoughEpoch
	for j := 0; j < int(r.RecordNumber); j++ {
		tmpRecord.Epoch[j] = r.NeedRecordEpoch[i].Epoch

		tmpRecord.BaseLineReward[j] = ToFile(r.NeedRecordEpoch[i].BaseLineReward)
		tmpRecord.SimpleReward[j] = ToFile(r.NeedRecordEpoch[i].SimpleReward)
		tmpRecord.EpochReward[j] = ToFile(r.NeedRecordEpoch[i].EpochReward)
		tmpRecord.InvestorAndProtoRelease[j] = ToFile(r.NeedRecordEpoch[i].InvestorAndProtoRelease)
		tmpRecord.NetworkCirculatingSupply[j] = ToFile(r.NeedRecordEpoch[i].NetworkCirculatingSupply)
		tmpRecord.NetWorkTotalReward[j] = ToFile(*r.NeedRecordEpoch[i].NetWorkTotalReward)
		tmpRecord.NetWorkTotalDeposit[j] = ToFile(*r.NeedRecordEpoch[i].NetWorkTotalDeposit)
		tmpRecord.NetWorkRewardLockFunds[j] = ToFile(*r.NeedRecordEpoch[i].NetWorkRewardLockFunds)
		tmpRecord.NetWorkDepositLockFunds[j] = ToFile(*r.NeedRecordEpoch[i].NetWorkDepositLockFunds)
		tmpRecord.NetWorkRewardUnLockFunds[j] = ToFile(*r.NeedRecordEpoch[i].NetWorkRewardUnLockFunds)
		tmpRecord.NetWorkDepositUnLockFunds[j] = ToFile(*r.NeedRecordEpoch[i].NetWorkDepositUnLockFunds)

		tmpRecord.CumSumRealized[j] = r.NeedRecordEpoch[i].CumSumRealized
		tmpRecord.CumSumBaselinePower[j] = r.NeedRecordEpoch[i].CumSumBaselinePower
		tmpRecord.EffectiveNetworkTime[j] = r.NeedRecordEpoch[i].EffectiveNetworkTime
		tmpRecord.TotalNetworkPower[j] = r.NeedRecordEpoch[i].TotalNetworkPower
		tmpRecord.BaseLinePower[j] = r.NeedRecordEpoch[i].BaseLinePower
		tmpRecord.EffectiveBaselinePower[j] = r.NeedRecordEpoch[i].EffectiveBaselinePower
		tmpRecord.IPFSMainAddPower[j] = r.NeedRecordEpoch[i].IPFSMainAddPower
		tmpRecord.IPFSMainTotalPower[j] = r.NeedRecordEpoch[i].IPFSMainTotalPower

		tmpRecord.IPFSMainExpectedReward[j] = ToFile(r.NeedRecordEpoch[i].IPFSMainExpectedReward)
		tmpRecord.IPFSMainTotalReward[j] = ToFile(*r.NeedRecordEpoch[i].IPFSMainTotalReward)
		tmpRecord.IPFSMainIP[j] = ToFile(r.NeedRecordEpoch[i].IPFSMainIP)
		tmpRecord.IPFSMainTotalDeposit[j] = ToFile(*r.NeedRecordEpoch[i].IPFSMainTotalDeposit)
		tmpRecord.IPFSMainRewardLockFunds[j] = ToFile(*r.NeedRecordEpoch[i].IPFSMainRewardLockFunds)
		tmpRecord.IPFSMainDepositLockFunds[j] = ToFile(*r.NeedRecordEpoch[i].IPFSMainDepositLockFunds)
		tmpRecord.IPFSMainRewardUnLockFunds[j] = ToFile(*r.NeedRecordEpoch[i].IPFSMainRewardUnLockFunds)
		tmpRecord.IPFSMainDepositUnLockFunds[j] = ToFile(*r.NeedRecordEpoch[i].IPFSMainDepositUnLockFunds)
		i++
	}
	return tmpRecord
}

func (r *Record) PrintRecordPoint() {
	tmpRecord := r.GetRecordPoint()
	log.Print("the CirculateEnoughEpoch is:")
	log.Println(r.CirculateEnoughEpoch)
	log.Print("the Epoch is:")
	log.Println(tmpRecord.Epoch)
	log.Print("the BaseLineReward is:")
	log.Println(tmpRecord.BaseLineReward)
	log.Print("the SimpleReward is:")
	log.Println(tmpRecord.SimpleReward)
	log.Print("the EpochReward is:")
	log.Println(tmpRecord.EpochReward)
	log.Print("the InvestorAndProtoRelease is:")
	log.Println(tmpRecord.InvestorAndProtoRelease)
	log.Print("the NetworkCirculatingSupply is:")
	log.Println(tmpRecord.NetworkCirculatingSupply)
	log.Print("the NetWorkTotalReward is:")
	log.Println(tmpRecord.NetWorkTotalReward)
	log.Print("the NetWorkTotalDeposit is:")
	log.Println(tmpRecord.NetWorkTotalDeposit)
	log.Print("the NetWorkRewardLockFunds is:")
	log.Println(tmpRecord.NetWorkRewardLockFunds)
	log.Print("the NetWorkDepositLockFunds is:")
	log.Println(tmpRecord.NetWorkDepositLockFunds)
	log.Print("the NetWorkRewardUnLockFunds is:")
	log.Println(tmpRecord.NetWorkRewardUnLockFunds)
	log.Print("the NetWorkDepositUnLockFunds is:")
	log.Println(tmpRecord.NetWorkDepositUnLockFunds)
	log.Print("the CumSumRealized is:")
	log.Println(tmpRecord.CumSumRealized)
	log.Print("the CumSumBaselinePower is:")
	log.Println(tmpRecord.CumSumBaselinePower)
	log.Print("the EffectiveNetworkTime is:")
	log.Println(tmpRecord.EffectiveNetworkTime)
	log.Print("the TotalNetworkPower is:")
	log.Println(tmpRecord.TotalNetworkPower)
	log.Print("the BaseLinePower is:")
	log.Println(tmpRecord.BaseLinePower)
	log.Print("the EffectiveBaselinePower is:")
	log.Println(tmpRecord.EffectiveBaselinePower)
	log.Print("the IPFSMainAddPower is:")
	log.Println(tmpRecord.IPFSMainAddPower)
	log.Print("the IPFSMainTotalPower is:")
	log.Println(tmpRecord.IPFSMainTotalPower)
	log.Print("the IPFSMainExpectedReward is:")
	log.Println(tmpRecord.IPFSMainExpectedReward)
	log.Print("the IPFSMainTotalReward is:")
	log.Println(tmpRecord.IPFSMainTotalReward)
	log.Print("the IPFSMainIP is:")
	log.Println(tmpRecord.IPFSMainIP)
	log.Print("the IPFSMainTotalDeposit is:")
	log.Println(tmpRecord.IPFSMainTotalDeposit)
	log.Print("the IPFSMainRewardLockFunds is:")
	log.Println(tmpRecord.IPFSMainRewardLockFunds)
	log.Print("the IPFSMainDepositLockFunds is:")
	log.Println(tmpRecord.IPFSMainDepositLockFunds)
	log.Print("the IPFSMainRewardUnLockFunds is:")
	log.Println(tmpRecord.IPFSMainRewardUnLockFunds)
	log.Print("the IPFSMainDepositUnLockFunds is:")
	log.Println(tmpRecord.IPFSMainDepositUnLockFunds)
}

func ToFile(value abi.TokenAmount) float64 {
	tmpValue := big2.Rat{}
	tmpValue.SetFrac(value.Int, big2.NewInt(1e18))
	f, _ := tmpValue.Float64()
	return f
}

func SimulateExecuteEconomyModel(epoch, startEpoch abi.ChainEpoch, pointNumber int, netWorkOneDayAddPower, IPFSMainOneDayAddPower abi.StoragePower) {
	step := (epoch - startEpoch) / abi.ChainEpoch(pointNumber)
	state := State{
		CumsumBaseline:         big.Zero(),
		CumsumRealized:         big.Zero(),
		EffectiveNetworkTime:   0,
		EffectiveBaselinePower: BaselineInitialValue,
		ThisEpochReward:        big.Zero(),
		ThisEpochBaselinePower: InitBaselinePower(),
		Epoch:                  -1,
		Record:                 NewRewardRecord(abi.ChainEpoch(startEpoch), abi.ChainEpoch(step), int64(pointNumber)),
	}
	//genesisPower 720T
	genesisPower := big.Lsh(big.NewInt(720), 40)
	state.updateToNextEpochWithRewardForTest(big.NewInt(0), big.NewInt(0), genesisPower)

	TotalOneEpochAdd := big.Div(netWorkOneDayAddPower,big.NewInt(builtin.EpochsInDay))
	IPFSMainEpochAddPower :=  big.Div(IPFSMainOneDayAddPower,big.NewInt(builtin.EpochsInDay))
	currentEpochRealizedPower := genesisPower
	i := abi.ChainEpoch(0)
	for i = 0; i < epoch; i++ {
		currentEpochRealizedPower = big.Add(TotalOneEpochAdd, currentEpochRealizedPower)
		state.updateToNextEpochWithRewardForTest(TotalOneEpochAdd, IPFSMainEpochAddPower, currentEpochRealizedPower)
	}
	state.Record.SaveToFile()
}
