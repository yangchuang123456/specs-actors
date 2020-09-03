package reward

import (
	"fmt"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
	"io"
	"sort"
)

// VestingFunds represents the vesting table state for the miner.
// It is a slice of (VestingEpoch, VestingAmount).
// The slice will always be sorted by the VestingEpoch.
type VestingFunds struct {
	Funds []VestingFund
}

func (v *VestingFunds) unlockVestedFunds(currEpoch abi.ChainEpoch) abi.TokenAmount {
	amountUnlocked := abi.NewTokenAmount(0)

	lastIndexToRemove := -1
	for i, vf := range v.Funds {
		if vf.Epoch >= currEpoch {
			break
		}

		amountUnlocked = big.Add(amountUnlocked, vf.Amount)
		lastIndexToRemove = i
	}

	// remove all entries upto and including lastIndexToRemove
	if lastIndexToRemove != -1 {
		v.Funds = v.Funds[lastIndexToRemove+1:]
	}

	return amountUnlocked
}

func (v *VestingFunds) addLockedFunds(currEpoch abi.ChainEpoch, vestingSum abi.TokenAmount,
	provingPeriodStart abi.ChainEpoch, spec *VestSpec) {
	// maps the epochs in VestingFunds to their indices in the slice
	epochToIndex := make(map[abi.ChainEpoch]int, len(v.Funds))
	for i, vf := range v.Funds {
		epochToIndex[vf.Epoch] = i
	}

	// Quantization is aligned with when regular cron will be invoked, in the last epoch of deadlines.
	vestBegin := currEpoch + spec.InitialDelay // Nothing unlocks here, this is just the start of the clock.
	vestPeriod := big.NewInt(int64(spec.VestPeriod))
	vestedSoFar := big.Zero()
	for e := vestBegin + spec.StepDuration; vestedSoFar.LessThan(vestingSum); e += spec.StepDuration {
		vestEpoch := quantizeUp(e, spec.Quantization, provingPeriodStart)
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

		// epoch already exists. Load existing entry
		// and update amount.
		if index, ok := epochToIndex[vestEpoch]; ok {
			currentAmt := v.Funds[index].Amount
			v.Funds[index].Amount = big.Add(currentAmt, vestThisTime)
		} else {
			// append a new entry -> slice will be sorted by epoch later.
			entry := VestingFund{Epoch: vestEpoch, Amount: vestThisTime}
			v.Funds = append(v.Funds, entry)
			epochToIndex[vestEpoch] = len(v.Funds) - 1
		}
	}

	// sort slice by epoch
	sort.Slice(v.Funds, func(first, second int) bool {
		return v.Funds[first].Epoch < v.Funds[second].Epoch
	})
}

func (v *VestingFunds) unlockUnvestedFunds(currEpoch abi.ChainEpoch, target abi.TokenAmount) abi.TokenAmount {
	amountUnlocked := abi.NewTokenAmount(0)
	lastIndexToRemove := -1
	startIndexForRemove := 0

	// retain funds that should have vested and unlock unvested funds
	for i, vf := range v.Funds {
		if amountUnlocked.GreaterThanEqual(target) {
			break
		}

		if vf.Epoch >= currEpoch {
			unlockAmount := big.Min(big.Sub(target, amountUnlocked), vf.Amount)
			amountUnlocked = big.Add(amountUnlocked, unlockAmount)
			newAmount := big.Sub(vf.Amount, unlockAmount)

			if newAmount.IsZero() {
				lastIndexToRemove = i
			} else {
				v.Funds[i].Amount = newAmount
			}
		} else {
			startIndexForRemove = i + 1
		}
	}

	// remove all entries in [startIndexForRemove, lastIndexToRemove]
	if lastIndexToRemove != -1 {
		v.Funds = append(v.Funds[0:startIndexForRemove], v.Funds[lastIndexToRemove+1:]...)
	}

	return amountUnlocked
}

// VestingFund represents miner funds that will vest at the given epoch.
type VestingFund struct {
	Epoch  abi.ChainEpoch
	Amount abi.TokenAmount
}

// ConstructVestingFunds constructs empty VestingFunds state.
func ConstructVestingFunds() *VestingFunds {
	v := new(VestingFunds)
	v.Funds = nil
	return v
}

var lengthBufVestingFunds = []byte{129}

func (t *VestingFunds) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}
	if _, err := w.Write(lengthBufVestingFunds); err != nil {
		return err
	}

	scratch := make([]byte, 9)

	// t.Funds ([]miner.VestingFund) (slice)
	if len(t.Funds) > cbg.MaxLength {
		return xerrors.Errorf("Slice value in field t.Funds was too long")
	}

	if err := cbg.WriteMajorTypeHeaderBuf(scratch, w, cbg.MajArray, uint64(len(t.Funds))); err != nil {
		return err
	}
	for _, v := range t.Funds {
		if err := v.MarshalCBOR(w); err != nil {
			return err
		}
	}
	return nil
}

func (t *VestingFunds) UnmarshalCBOR(r io.Reader) error {
	*t = VestingFunds{}

	br := cbg.GetPeeker(r)
	scratch := make([]byte, 8)

	maj, extra, err := cbg.CborReadHeaderBuf(br, scratch)
	if err != nil {
		return err
	}
	if maj != cbg.MajArray {
		return fmt.Errorf("cbor input should be of type array")
	}

	if extra != 1 {
		return fmt.Errorf("cbor input had wrong number of fields")
	}

	// t.Funds ([]miner.VestingFund) (slice)

	maj, extra, err = cbg.CborReadHeaderBuf(br, scratch)
	if err != nil {
		return err
	}

	if extra > cbg.MaxLength {
		return fmt.Errorf("t.Funds: array too large (%d)", extra)
	}

	if maj != cbg.MajArray {
		return fmt.Errorf("expected cbor array")
	}

	if extra > 0 {
		t.Funds = make([]VestingFund, extra)
	}

	for i := 0; i < int(extra); i++ {

		var v VestingFund
		if err := v.UnmarshalCBOR(br); err != nil {
			return err
		}

		t.Funds[i] = v
	}

	return nil
}

var lengthBufVestingFund = []byte{130}

func (t *VestingFund) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}
	if _, err := w.Write(lengthBufVestingFund); err != nil {
		return err
	}

	scratch := make([]byte, 9)

	// t.Epoch (abi.ChainEpoch) (int64)
	if t.Epoch >= 0 {
		if err := cbg.WriteMajorTypeHeaderBuf(scratch, w, cbg.MajUnsignedInt, uint64(t.Epoch)); err != nil {
			return err
		}
	} else {
		if err := cbg.WriteMajorTypeHeaderBuf(scratch, w, cbg.MajNegativeInt, uint64(-t.Epoch-1)); err != nil {
			return err
		}
	}

	// t.Amount (big.Int) (struct)
	if err := t.Amount.MarshalCBOR(w); err != nil {
		return err
	}
	return nil
}

func (t *VestingFund) UnmarshalCBOR(r io.Reader) error {
	*t = VestingFund{}

	br := cbg.GetPeeker(r)
	scratch := make([]byte, 8)

	maj, extra, err := cbg.CborReadHeaderBuf(br, scratch)
	if err != nil {
		return err
	}
	if maj != cbg.MajArray {
		return fmt.Errorf("cbor input should be of type array")
	}

	if extra != 2 {
		return fmt.Errorf("cbor input had wrong number of fields")
	}

	// t.Epoch (abi.ChainEpoch) (int64)
	{
		maj, extra, err := cbg.CborReadHeaderBuf(br, scratch)
		var extraI int64
		if err != nil {
			return err
		}
		switch maj {
		case cbg.MajUnsignedInt:
			extraI = int64(extra)
			if extraI < 0 {
				return fmt.Errorf("int64 positive overflow")
			}
		case cbg.MajNegativeInt:
			extraI = int64(extra)
			if extraI < 0 {
				return fmt.Errorf("int64 negative oveflow")
			}
			extraI = -1 - extraI
		default:
			return fmt.Errorf("wrong type for int64 field: %d", maj)
		}

		t.Epoch = abi.ChainEpoch(extraI)
	}
	// t.Amount (big.Int) (struct)

	{

		if err := t.Amount.UnmarshalCBOR(br); err != nil {
			return xerrors.Errorf("unmarshaling t.Amount: %w", err)
		}

	}
	return nil
}


