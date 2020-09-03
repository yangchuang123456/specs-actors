package main

import (
	"errors"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/reward"
	"github.com/urfave/cli"
	_ "net/http/pprof"
	"os"
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
		Usage: "net work add power one day,default:15P",
		Value: big.Lsh(big.NewInt(15), 50).Int64(),
	}
	IPFSMainOneDayAddPowerFlag = cli.Int64Flag{
		Name:  "IPFSMainOneDayAddPower",
		Usage: "ipfsMain add power one day,default:1.5P",
		Value: big.Lsh(big.NewInt(1536), 40).Int64(),
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
		if epoch<=10{
			return errors.New("the EpochEnd should be larger than 10")
		}
		recordStart:= abi.ChainEpoch(c.GlobalUint64("RecordStartEpoch"))
		pointNumber:= abi.ChainEpoch(c.GlobalUint64("RecordEpochNumber"))
		netWorkOneDayAddPower := c.GlobalInt64("NetWorkOneDayAddPower")
		ipfsMainOneDayAddPower := c.GlobalInt64("IPFSMainOneDayAddPower")
		//log.Println("the ipfsMainOneDayAddPower is:",ipfsMainOneDayAddPower)
		reward.SimulateExecuteEconomyModel(epoch,recordStart,int(pointNumber),big.NewInt(netWorkOneDayAddPower),big.NewInt(ipfsMainOneDayAddPower))
		return nil
	}
	app.Run(os.Args)
}
