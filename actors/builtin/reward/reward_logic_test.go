package reward

import (
	"bytes"
	"fmt"
	"log"
	gbig "math/big"
	"testing"

	abi "github.com/filecoin-project/specs-actors/actors/abi"
	big "github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/stretchr/testify/assert"
	"github.com/xorcare/golden"
)

func TestComputeRTeta(t *testing.T) {
	baselinePowerAt := func(epoch abi.ChainEpoch) abi.StoragePower {
		return big.Mul(big.NewInt(int64(epoch+1)), big.NewInt(2048))
	}

	assert.Equal(t, 0.5, q128ToF(computeRTheta(1, baselinePowerAt(1), big.NewInt(2048+2*2048*0.5), big.NewInt(2048+2*2048))))
	assert.Equal(t, 0.25, q128ToF(computeRTheta(1, baselinePowerAt(1), big.NewInt(2048+2*2048*0.25), big.NewInt(2048+2*2048))))

	cumsum15 := big.NewInt(0)
	for i := abi.ChainEpoch(0); i < 16; i++ {
		cumsum15 = big.Add(cumsum15, baselinePowerAt(i))
	}
	assert.Equal(t, 15.25, q128ToF(computeRTheta(16,
		baselinePowerAt(16),
		big.Add(cumsum15, big.Div(baselinePowerAt(16), big.NewInt(4))),
		big.Add(cumsum15, baselinePowerAt(16)))))
}

func TestBaselineReward(t *testing.T) {
	step := gbig.NewInt(5000)
	step = step.Lsh(step, precision)
	step = step.Sub(step, gbig.NewInt(77777777777)) // offset from full integers

	delta := gbig.NewInt(1)
	delta = delta.Lsh(delta, precision)
	delta = delta.Sub(delta, gbig.NewInt(33333333333)) // offset from full integers

	prevTheta := new(gbig.Int)
	theta := new(gbig.Int).Set(delta)

	b := &bytes.Buffer{}
	b.WriteString("t0, t1, y\n")
	simple := computeReward(0, big.Zero(), big.Zero())

	for i := 0; i < 512; i++ {
		reward := computeReward(0, big.Int{Int: prevTheta}, big.Int{Int: theta})
		reward = big.Sub(reward, simple)
		fmt.Fprintf(b, "%s,%s,%s\n", prevTheta, theta, reward.Int)
		prevTheta = prevTheta.Add(prevTheta, step)
		theta = theta.Add(theta, step)
	}

	golden.Assert(t, b.Bytes())
}

func TestSimpleRewrad(t *testing.T) {
	b := &bytes.Buffer{}
	b.WriteString("x, y\n")
	for i := int64(0); i < 512; i++ {
		x := i * 5000
		reward := computeReward(abi.ChainEpoch(x), big.Zero(), big.Zero())
		fmt.Fprintf(b, "%d,%s\n", x, reward.Int)
	}

	golden.Assert(t, b.Bytes())
}

func TestBaselineRewardGrowth(t *testing.T) {

	baselineInYears := func(start abi.StoragePower, x abi.ChainEpoch) abi.StoragePower {
		baseline := start
		for i := abi.ChainEpoch(0); i < x*builtin.EpochsInYear; i++ {
			baseline = BaselinePowerFromPrev(baseline)
		}
		return baseline
	}

	// Baseline reward should have 200% growth rate
	// This implies that for every year x, the baseline function should be:
	// StartVal * 3^x.
	//
	// Error values for 1 years of growth were determined empirically with latest
	// baseline power construction to set bounds in this test in order to
	// 1. throw a test error if function changes and percent error goes up
	// 2. serve as documentation of current error bounds
	type growthTestCase struct {
		StartVal abi.StoragePower
		ErrBound float64
	}
	cases := []growthTestCase{
		// 1 byte
		{
			abi.NewStoragePower(1),
			1,
		},
		// GiB
		{
			abi.NewStoragePower(1 << 30),
			1e-3,
		},
		// TiB
		{
			abi.NewStoragePower(1 << 40),
			1e-6,
		},
		// PiB
		{
			abi.NewStoragePower(1 << 50),
			1e-8,
		},
		// EiB
		{
			BaselineInitialValue,
			1e-8,
		},
		// ZiB
		{
			big.Lsh(big.NewInt(1), 70),
			1e-8,
		},
		// non power of 2 ~ 1 EiB
		{
			abi.NewStoragePower(513633559722596517),
			1e-8,
		},
	}
	for _, testCase := range cases {
		years := int64(1)
		log.Println("baselineInYears startVal is:", testCase.StartVal)
		end := baselineInYears(testCase.StartVal, abi.ChainEpoch(1))
		log.Println("baselineInYears end is:", end)
		multiplier := big.Exp(big.NewInt(3), big.NewInt(years)) // keeping this generalized in case we want to test more years
		log.Println("the multiplier is:", multiplier)
		expected := big.Mul(testCase.StartVal, multiplier)
		log.Println("the expected is:", expected)

		diff := big.Sub(expected, end)

		perrFrac := gbig.NewRat(1, 1).SetFrac(diff.Int, expected.Int)
		perr, _ := perrFrac.Float64()
		log.Println("the perr is:", perr)
		log.Println("")
		assert.Less(t, perr, testCase.ErrBound)
	}
}

func Test_baselineGrow(t *testing.T) {
	baselineInYears := func(start abi.StoragePower, x abi.ChainEpoch) abi.StoragePower {
		baseline := start
		for i := abi.ChainEpoch(0); i < x*builtin.EpochsInYear; i++ {
			baseline = BaselinePowerFromPrev(baseline)
			//log.Println("the baseline is:",baseline)
		}
		return baseline
	}
	startPower := InitBaselinePower()
	log.Println("the BaselineInitialValue is:", BaselineInitialValue)
	log.Println("the startPower is:", startPower)
	end := baselineInYears(startPower, 1)
	log.Println("the end is:", end)
	log.Println("the end/start is:", big.Div(end, startPower))
}

func Test_BaselinePowerNextEpoch(t *testing.T) {
	log.Println("the BaselineInitialValue is:", BaselineInitialValue)
	rat := gbig.NewRat(1, 1)
	precision := big.Lsh(big.NewInt(1), 128)
	rat.SetFrac(BaselineExponent.Int, precision.Int)
	f, _ := rat.Float64()
	log.Println("the BaselineExponent is:", f)
	nextEpoch := BaselinePowerFromPrev(BaselineInitialValue)
	log.Println("the nex epoch baseline power is:", nextEpoch)
	log.Println("the base total supply is:", big.NewInt(900e6))
	log.Println("the 1e-3 is:", 1e-3)
	st := State{}
	st.Print()
	//reward.BaselinePowerNextEpoch(abi.StoragePower{})
}

func Test_BaseLinePower(t *testing.T) {
	//addEpochPower := big.Lsh(big.NewInt(2), 40)
	onDayAddEpoch := big.Lsh(big.NewInt(20), 50)
	addEpochPower := big.Div(onDayAddEpoch, big.NewInt(builtin.EpochsInDay))

	baseLinePower := big.Lsh(big.NewInt(2), 60)
	totalPower := big.NewInt(0)
	currentEpochPower := big.NewInt(4096)
	//currentBaselineValue := reward.BaselineInitialValue
	for i := 0; ; i++ {
		currentEpochPower = big.Add(addEpochPower, currentEpochPower)
		//if currentEpochPower.Cmp()
		totalPower = big.Add(totalPower, currentEpochPower)
		if totalPower.Cmp(baseLinePower.Int) >= 0 {
			log.Println("arrive base line", i)
			return
		}
	}
}

func Test_EconomyModel(t *testing.T) {
	//epoch := builtin.EpochsInDay * 365
	epoch := builtin.EpochsInDay*100
	//epoch := 100
	state := State{
		CumsumBaseline:         big.Zero(),
		CumsumRealized:         big.Zero(),
		EffectiveNetworkTime:   0,
		EffectiveBaselinePower: BaselineInitialValue,
		ThisEpochReward:        big.Zero(),
		ThisEpochBaselinePower: InitBaselinePower(),
		Epoch:                  -1,
		Record:                 NewRewardRecord(int64(epoch + 1)),
	}
	//genesisPower 720T
	genesisPower := big.Lsh(big.NewInt(720), 40)
	//state.updateToNextEpoch(genesisPower)
	log.Print(state)
	//state.Print()
	state.updateToNextEpochWithRewardForTest(big.NewInt(0), genesisPower)
	log.Print(state)
	//network add 10 PB one day
	TotalOneDayAdd := big.Lsh(big.NewInt(10), 50)
	TotalOneEpochAdd := big.Div(TotalOneDayAdd, big.NewInt(builtin.EpochsInDay))
	//addEpochPower := big.Lsh(big.NewInt(2), 40)
	//totalPower := big.NewInt(0)
	//epoch := builtin.EpochsInYear

	//IPFSMain add %5 totalAddPower one day
	//IPFSMainOnHourAddEpoch := big.Div(big.Mul(TotalOneEpochAdd,big.NewInt(5)),big.NewInt(100))
	IPFSMainEpochAddPower := big.Div(big.Mul(TotalOneEpochAdd,big.NewInt(5)),big.NewInt(100))

	currentEpochRealizedPower := genesisPower
	i := 0
	for i = 0; i < epoch; i++ {
		currentEpochRealizedPower = big.Add(TotalOneEpochAdd, currentEpochRealizedPower)
		state.updateToNextEpochWithRewardForTest(TotalOneEpochAdd, currentEpochRealizedPower)
		state.paddingIPFSMain(IPFSMainEpochAddPower)
	}
	log.Println("the point is:", )
	state.Record.PrintPointAccordingToPointNumber(10)
	state.Record.SaveToFile()
	//pointNumber := 10
}
