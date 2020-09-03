package main

import (
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/reward"
	"github.com/urfave/cli"
	"os"
 	_ "net/http/pprof"
)

var (
	EpochFlag = cli.Uint64Flag{
		Name:  "EpochEnd",
		Usage: "set simulation end epoch",
		Value: builtin.EpochsInHour,
	}
	RecordStartEpochFlag = cli.Uint64Flag{
		Name:  "RecordStartEpoch",
		Usage: "set record start epoch",
		Value: 1,
	}
	RecordEpochNumberFlag = cli.Uint64Flag{
		Name:  "RecordEpochNumber",
		Usage: "record epoch point number",
		Value: 10,
	}
	NetWorkOneDayAddPowerFlag = cli.Int64Flag{
		Name:  "NetWorkOneDayAddPower",
		Usage: "net work add power one day,default:10P",
		Value: big.Lsh(big.NewInt(10), 50).Int64(),
	}
	IPFSMainOneDayAddPowerFlag = cli.Int64Flag{
		Name:  "IPFSMainOneDayAddPower",
		Usage: "ipfsMain add power one day,default:512T",
		Value: big.Lsh(big.NewInt(512), 40).Int64(),
	}
)

func main() {
	app := cli.NewApp()
	app.Flags = []cli.Flag {
		EpochFlag,
		RecordStartEpochFlag,
		RecordEpochNumberFlag,
		NetWorkOneDayAddPowerFlag,
		IPFSMainOneDayAddPowerFlag,
	}
	app.Action = func(c *cli.Context) error {
		epoch:= abi.ChainEpoch(c.GlobalUint64("EpochEnd"))
		recordStart:= abi.ChainEpoch(c.GlobalUint64("RecordStartEpoch"))
		pointNumber:= abi.ChainEpoch(c.GlobalUint64("RecordEpochNumber"))
		netWorkOneDayAddPower := c.GlobalInt64("NetWorkOneDayAddPower")
		ipfsMainOneDayAddPower := c.GlobalInt64("IPFSMainOneDayAddPower")

		reward.SimulateExecuteEconomyModel(epoch,recordStart,int(pointNumber),big.NewInt(netWorkOneDayAddPower),big.NewInt(ipfsMainOneDayAddPower))
		return nil
	}
	app.Run(os.Args)
}
