package log

import 	logging "github.com/ipfs/go-log/v2"

var Log = logging.Logger("economy-model")

func init(){
	logging.SetLogLevel("economy-model","DEBUG")
}
