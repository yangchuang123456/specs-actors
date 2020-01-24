package mock

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"testing"

	addr "github.com/filecoin-project/go-address"
	cid "github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	"github.com/filecoin-project/specs-actors/actors/runtime/indices"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
)

// A mock runtime for unit testing of actors in isolation.
// The mock allows direct specification of the runtime context as observable by an actor, supports
// the storage interface, and mocks out side-effect-inducing calls.
type Runtime struct {
	// Execution context
	ctx           context.Context
	epoch         abi.ChainEpoch
	receiver      addr.Address
	caller        addr.Address
	callerType    cid.Cid
	miner         addr.Address
	valueReceived abi.TokenAmount

	// Actor state
	state   cid.Cid
	balance abi.TokenAmount

	// VM implementation
	store         map[cid.Cid][]byte
	inTransaction bool

	// Expectations
	t                        *testing.T
	expectValidateCallerAny  bool
	expectValidateCallerAddr []addr.Address
	expectValidateCallerType []cid.Cid
}

var _ runtime.Runtime = &Runtime{}
var _ runtime.StateHandle = &Runtime{}

var cidBuilder = cid.V1Builder{
	Codec:    cid.DagCBOR,
	MhType:   mh.SHA2_256,
	MhLength: 0, // default
}

///// Implementation of the runtime API /////

func (rt *Runtime) CurrEpoch() abi.ChainEpoch {
	return rt.epoch
}

func (rt *Runtime) CurrReceiver() addr.Address {
	return rt.receiver
}

func (rt *Runtime) ImmediateCaller() addr.Address {
	return rt.caller
}

func (rt *Runtime) ValidateImmediateCallerAcceptAny() {
	if !rt.expectValidateCallerAny {
		rt.t.Fatalf("unexpected validate-caller-any")
	}
	rt.expectValidateCallerAny = false
}

func (rt *Runtime) ValidateImmediateCallerIs(addrs ...addr.Address) {
	// Check and clear expectations.
	if len(rt.expectValidateCallerAddr) == 0 {
		rt.t.Errorf("unexpected validate caller addrs")
		return
	}
	if !reflect.DeepEqual(rt.expectValidateCallerAddr, addrs) {
		rt.t.Errorf("unexpected validate caller addrs %v, expected %v", addrs, rt.expectValidateCallerAddr)
		return
	}
	defer func() {
		rt.expectValidateCallerAddr = []addr.Address{}
	}()

	// Implement method.
	for _, expected := range addrs {
		if rt.caller == expected {
			return
		}
	}
	rt.Abort(exitcode.ErrForbidden, "caller address %v forbidden, allowed: %v", rt.caller, addrs)
}

func (rt *Runtime) ValidateImmediateCallerType(types ...cid.Cid) {
	// Check and clear expectations.
	if len(rt.expectValidateCallerType) == 0 {
		rt.t.Errorf("unexpected validate caller code")
	}
	if !reflect.DeepEqual(rt.expectValidateCallerType, types) {
		rt.t.Errorf("unexpected validate caller code %v, expected %v", types, rt.expectValidateCallerType)
	}
	defer func() {
		rt.expectValidateCallerType = []cid.Cid{}
	}()

	// Implement method.
	for _, expected := range types {
		if rt.callerType == expected {
			return
		}
	}
	rt.Abort(exitcode.ErrForbidden, "caller type %v forbidden, allowed: %v", rt.callerType, types)
}

func (rt *Runtime) ToplevelBlockWinner() addr.Address {
	return rt.miner
}

func (rt *Runtime) CurrentBalance() abi.TokenAmount {
	return rt.balance
}

func (rt *Runtime) ValueReceived() abi.TokenAmount {
	return rt.valueReceived
}

func (rt *Runtime) GetActorCodeID(addr addr.Address) (ret cid.Cid, ok bool) {
	panic("implement me")
}

func (rt *Runtime) GetRandomness(epoch abi.ChainEpoch) abi.RandomnessSeed {
	panic("implement me")
}

func (rt *Runtime) State() runtime.StateHandle {
	return rt
}

func (rt *Runtime) IpldGet(c cid.Cid, o runtime.CBORUnmarshaler) bool {
	data, found := rt.store[c]
	if found {
		err := o.UnmarshalCBOR(bytes.NewReader(data))
		if err != nil {
			rt.Abort(exitcode.SysErrSerialization, err.Error())
		}
	}
	return found
}

func (rt *Runtime) IpldPut(o runtime.CBORMarshaler) cid.Cid {
	r := bytes.Buffer{}
	err := o.MarshalCBOR(&r)
	if err != nil {
		rt.Abort(exitcode.SysErrSerialization, err.Error())
	}
	data := r.Bytes()
	key, err := cidBuilder.Sum(data)
	if err != nil {
		rt.Abort(exitcode.SysErrSerialization, err.Error())
	}
	rt.store[key] = data
	return key
}

func (rt *Runtime) Send(toAddr addr.Address, methodNum abi.MethodNum, params abi.MethodParams, value abi.TokenAmount) (runtime.SendReturn, exitcode.ExitCode) {
	if rt.inTransaction {
		rt.Abort(exitcode.SysErrorIllegalActor, "side-effect within transaction")
	}
	panic("implement me")
}

func (rt *Runtime) NewActorAddress() addr.Address {
	panic("implement me")
}

func (rt *Runtime) CreateActor(codeId cid.Cid, address addr.Address) {
	if rt.inTransaction {
		rt.Abort(exitcode.SysErrorIllegalActor, "side-effect within transaction")
	}
	panic("implement me")
}

func (rt *Runtime) DeleteActor(address addr.Address) {
	if rt.inTransaction {
		rt.Abort(exitcode.SysErrorIllegalActor, "side-effect within transaction")
	}
	panic("implement me")
}

func (rt *Runtime) CurrIndices() indices.Indices {
	panic("implement me")
}

func (rt *Runtime) Abort(errExitCode exitcode.ExitCode, msg string, args ...interface{}) {
	panic(abort{errExitCode, fmt.Sprintf(msg, args...)})
}

func (rt *Runtime) AbortStateMsg(msg string) {
	rt.Abort(exitcode.ErrPlaceholder, msg)
}

func (rt *Runtime) Syscalls() runtime.Syscalls {
	panic("implement me")
}

func (rt *Runtime) Context() context.Context {
	return rt.ctx
}

func (rt *Runtime) StartSpan(name string) runtime.TraceSpan {
	return &TraceSpan{}
}

///// State handle implementation /////

func (rt *Runtime) Construct(f func() runtime.CBORMarshaler) {
	if rt.state.Defined() {
		rt.Abort(exitcode.SysErrorIllegalActor, "state already constructed")
	}
	st := f()
	rt.state = rt.IpldPut(st)
}

func (rt *Runtime) Readonly(st runtime.CBORUnmarshaler) {
	found := rt.IpldGet(rt.state, st)
	if !found {
		rt.Abort(exitcode.SysErrInternal, "actor state not found: %v", rt.state)
	}
}

func (rt *Runtime) Transaction(st runtime.CBORer, f func() interface{}) interface{} {
	if rt.inTransaction {
		rt.Abort(exitcode.SysErrorIllegalActor, "nested transaction")
	}
	rt.Readonly(st)
	rt.inTransaction = true
	ret := f()
	rt.state = rt.IpldPut(st)
	rt.inTransaction = false
	return ret
}

///// Trace span implementation /////

type TraceSpan struct {
}

func (t TraceSpan) End() {
	// no-op
}

type abort struct {
	code exitcode.ExitCode
	msg  string
}

///// Inspection facilities /////

func (rt *Runtime) StateRoot() cid.Cid {
	return rt.state
}

func (rt *Runtime) GetState(o runtime.CBORUnmarshaler) {
	rt.IpldGet(rt.state, o)
}

func (rt *Runtime) Store() adt.Store {
	return adt.AsStore(rt)
}

///// Mocking facilities /////

func (rt *Runtime) ExpectValidateCallerAny() {
	rt.expectValidateCallerAny = true
}

func (rt *Runtime) ExpectValidateCallerAddr(addrs ...addr.Address) {
	rt.expectValidateCallerAddr = addrs[:]
}

func (rt *Runtime) ExpectValidateCallerType(types ...cid.Cid) {
	rt.expectValidateCallerType = types[:]
}

// Verifies that expected calls were received, and resets all expectations.
func (rt *Runtime) Verify() {
	if rt.expectValidateCallerAny {
		rt.t.Error("expected ValidateCallerAny, not received")
	}
	if len(rt.expectValidateCallerAddr) > 0 {
		rt.t.Errorf("expected ValidateCallerAddr %v, not received", rt.expectValidateCallerAddr)
	}
	if len(rt.expectValidateCallerType) > 0 {
		rt.t.Errorf("expected ValidateCallerType %v, not received", rt.expectValidateCallerType)
	}

	rt.Reset()
}

// Resets expectations
func (rt *Runtime) Reset() {
	rt.expectValidateCallerAny = false
	rt.expectValidateCallerAddr = nil
	rt.expectValidateCallerType = nil
}

// Calls f() expecting it to invoke Runtime.Abort() with a specified exit code.
func (rt *Runtime) ExpectAbort(expected exitcode.ExitCode, f func()) {
	defer func() {
		r := recover()
		a, ok := r.(abort)
		if !ok {
			panic(r)
		}
		if a.code != expected {
			rt.t.Errorf("abort expected code %v, got %v %s", expected, a.code, a.msg)
		}
	}()
	f()
	rt.t.Errorf("expected abort with code %v but call succeeded", expected)
}