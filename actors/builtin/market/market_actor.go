package market

import (
	"fmt"
	"sort"

	addr "github.com/filecoin-project/go-address"
	cbg "github.com/whyrusleeping/cbor-gen"
	xerrors "golang.org/x/xerrors"

	abi "github.com/filecoin-project/specs-actors/actors/abi"
	big "github.com/filecoin-project/specs-actors/actors/abi/big"
	builtin "github.com/filecoin-project/specs-actors/actors/builtin"
	verifreg "github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
	vmr "github.com/filecoin-project/specs-actors/actors/runtime"
	exitcode "github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	. "github.com/filecoin-project/specs-actors/actors/util"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
)

type Actor struct{}

type Runtime = vmr.Runtime

func (a Actor) Exports() []interface{} {
	return []interface{}{
		builtin.MethodConstructor: a.Constructor,
		2:                         a.AddBalance,
		3:                         a.WithdrawBalance,
		4:                         a.PublishStorageDeals,
		5:                         a.VerifyDealsForActivation,
		6:                         a.ActivateDeals,
		7:                         a.OnMinerSectorsTerminate,
		8:                         a.ComputeDataCommitment,
		9:                         a.CronTick,
	}
}

var _ abi.Invokee = Actor{}

////////////////////////////////////////////////////////////////////////////////
// Actor methods
////////////////////////////////////////////////////////////////////////////////

func (a Actor) Constructor(rt Runtime, _ *adt.EmptyValue) *adt.EmptyValue {
	rt.ValidateImmediateCallerIs(builtin.SystemActorAddr)

	emptyArray, err := adt.MakeEmptyArray(adt.AsStore(rt)).Root()
	if err != nil {
		rt.Abortf(exitcode.ErrIllegalState, "failed to create storage market state: %v", err)
	}

	emptyMap, err := adt.MakeEmptyMap(adt.AsStore(rt)).Root()
	if err != nil {
		rt.Abortf(exitcode.ErrIllegalState, "failed to create storage market state: %v", err)
	}

	emptyMSet, err := MakeEmptySetMultimap(adt.AsStore(rt)).Root()
	if err != nil {
		rt.Abortf(exitcode.ErrIllegalState, "failed to create storage market state: %v", err)
	}

	st := ConstructState(emptyArray, emptyMap, emptyMSet)
	rt.State().Create(st)
	return nil
}

type WithdrawBalanceParams struct {
	ProviderOrClientAddress addr.Address
	Amount                  abi.TokenAmount
}

// Attempt to withdraw the specified amount from the balance held in escrow.
// If less than the specified amount is available, yields the entire available balance.
func (a Actor) WithdrawBalance(rt Runtime, params *WithdrawBalanceParams) *adt.EmptyValue {
	if params.Amount.LessThan(big.Zero()) {
		rt.Abortf(exitcode.ErrIllegalArgument, "negative amount %v", params.Amount)
	}
	// withdrawal can ONLY be done by a signing party.
	rt.ValidateImmediateCallerType(builtin.CallerTypesSignable...)

	nominal, recipient, approvedCallers := escrowAddress(rt, params.ProviderOrClientAddress)
	// for providers -> only corresponding owner or worker can withdraw
	// for clients -> only the client i.e the recipient can withdraw
	rt.ValidateImmediateCallerIs(approvedCallers...)

	amountExtracted := abi.NewTokenAmount(0)
	var st State
	rt.State().Transaction(&st, func() interface{} {
		msm, err := st.mutator(adt.AsStore(rt)).withEscrowTable(WritePermission).
			withLockedTable(WritePermission).build()
		builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to load state")

		// The withdrawable amount might be slightly less than nominal
		// depending on whether or not all relevant entries have been processed
		// by cron
		minBalance, err, code := getBalance(msm.lockedTable, nominal)
		builtin.RequireNoErr(rt, err, code, "failed to get locked balance")

		ex, err := msm.escrowTable.SubtractWithMinimum(nominal, params.Amount, minBalance)
		builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to subtract form escrow table")

		// remove account if escrow balance is now zero
		err, code = msm.removeAccountIfNoBalance(nominal)
		builtin.RequireNoErr(rt, err, code, "failed to remove balance")

		err = msm.commitState()
		builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to flush state")

		amountExtracted = ex
		return nil
	})

	_, code := rt.Send(recipient, builtin.MethodSend, nil, amountExtracted)
	builtin.RequireSuccess(rt, code, "failed to send funds")
	return nil
}

// Deposits the received value into the balance held in escrow.
func (a Actor) AddBalance(rt Runtime, providerOrClientAddress *addr.Address) *adt.EmptyValue {
	// only signing parties can add balance for client AND provider.
	rt.ValidateImmediateCallerType(builtin.CallerTypesSignable...)

	msgValue := rt.Message().ValueReceived()

	nominal, _, _ := escrowAddress(rt, *providerOrClientAddress)

	var st State
	rt.State().Transaction(&st, func() interface{} {
		msm, err := st.mutator(adt.AsStore(rt)).withEscrowTable(WritePermission).
			withLockedTable(WritePermission).build()
		builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to load state")

		err = msm.escrowTable.AddCreate(nominal, msgValue)
		builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to add balance to escrow table")

		err = msm.lockedTable.AddCreate(nominal, big.Zero())
		builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to add locked balance")

		err = msm.commitState()
		builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to flush state")

		return nil
	})
	return nil
}

type PublishStorageDealsParams struct {
	Deals []ClientDealProposal
}

type PublishStorageDealsReturn struct {
	IDs []abi.DealID
}

// Publish a new set of storage deals (not yet included in a sector).
func (a Actor) PublishStorageDeals(rt Runtime, params *PublishStorageDealsParams) *PublishStorageDealsReturn {

	// Deal message must have a From field identical to the provider of all the deals.
	// This allows us to retain and verify only the client's signature in each deal proposal itself.
	rt.ValidateImmediateCallerType(builtin.CallerTypesSignable...)
	if len(params.Deals) == 0 {
		rt.Abortf(exitcode.ErrIllegalArgument, "empty deals parameter")
	}

	// All deals should have the same provider so get worker once
	providerRaw := params.Deals[0].Proposal.Provider
	provider, ok := rt.ResolveAddress(providerRaw)
	if !ok {
		rt.Abortf(exitcode.ErrNotFound, "failed to resolve provider address %v", providerRaw)
	}

	codeID, ok := rt.GetActorCodeCID(provider)
	builtin.RequireParam(rt, ok, "no codeId for address %v", provider)
	if !codeID.Equals(builtin.StorageMinerActorCodeID) {
		rt.Abortf(exitcode.ErrIllegalArgument, "deal provider is not a StorageMinerActor")
	}

	_, worker := builtin.RequestMinerControlAddrs(rt, provider)
	if worker != rt.Message().Caller() {
		rt.Abortf(exitcode.ErrForbidden, "caller is not provider %v", provider)
	}

	for _, deal := range params.Deals {
		// Check VerifiedClient allowed cap and deduct PieceSize from cap.
		// Either the DealSize is within the available DataCap of the VerifiedClient
		// or this message will fail. We do not allow a deal that is partially verified.
		if deal.Proposal.VerifiedDeal {
			_, code := rt.Send(
				builtin.VerifiedRegistryActorAddr,
				builtin.MethodsVerifiedRegistry.UseBytes,
				&verifreg.UseBytesParams{
					Address:  deal.Proposal.Client,
					DealSize: big.NewIntUnsigned(uint64(deal.Proposal.PieceSize)),
				},
				abi.NewTokenAmount(0),
			)
			builtin.RequireSuccess(rt, code, "failed to add verified deal for client: %v", deal.Proposal.Client)
		}
	}

	var newDealIds []abi.DealID
	var st State
	rt.State().Transaction(&st, func() interface{} {
		msm, err := st.mutator(adt.AsStore(rt)).withPendingProposals(WritePermission).
			withDealProposals(WritePermission).withDealsByEpoch(WritePermission).withEscrowTable(WritePermission).
			withLockedTable(WritePermission).build()
		builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to load state")

		// All storage dealProposals will be added in an atomic transaction; this operation will be unrolled if any of them fails.
		for di, deal := range params.Deals {
			validateDeal(rt, deal)

			if deal.Proposal.Provider != provider && deal.Proposal.Provider != providerRaw {
				rt.Abortf(exitcode.ErrIllegalArgument, "cannot publish deals from different providers at the same time")
			}

			client, ok := rt.ResolveAddress(deal.Proposal.Client)
			if !ok {
				rt.Abortf(exitcode.ErrNotFound, "failed to resolve client address %v", deal.Proposal.Client)
			}
			// Normalise provider and client addresses in the proposal stored on chain (after signature verification).
			deal.Proposal.Provider = provider
			deal.Proposal.Client = client

			err, code := msm.lockClientAndProviderBalances(&deal.Proposal)
			builtin.RequireNoErr(rt, err, code, "failed to lock balance")

			id := msm.generateStorageDealID()

			pcid, err := deal.Proposal.Cid()
			builtin.RequireNoErr(rt, err, exitcode.ErrIllegalArgument, "failed to take cid of proposal %d", di)

			has, err := msm.pendingDeals.Get(adt.CidKey(pcid), nil)
			builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to check for existence of deal proposal")
			if has {
				rt.Abortf(exitcode.ErrIllegalArgument, "cannot publish duplicate deals")
			}

			err = msm.pendingDeals.Put(adt.CidKey(pcid), &deal.Proposal)
			builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to set pending deal")

			err = msm.dealProposals.Set(id, &deal.Proposal)
			builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to set deal")

			err = msm.dealsByEpoch.Put(deal.Proposal.StartEpoch, id)
			builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to set deal ops by epoch")

			newDealIds = append(newDealIds, id)
		}

		err = msm.commitState()
		builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to flush state")

		return nil
	})

	return &PublishStorageDealsReturn{newDealIds}
}

type VerifyDealsForActivationParams struct {
	DealIDs      []abi.DealID
	SectorExpiry abi.ChainEpoch
	SectorStart  abi.ChainEpoch
}

type VerifyDealsForActivationReturn struct {
	DealWeight         abi.DealWeight
	VerifiedDealWeight abi.DealWeight
}

// Verify that a given set of storage deals is valid for a sector currently being PreCommitted
// and return DealWeight of the set of storage deals given.
// The weight is defined as the sum, over all deals in the set, of the product of deal size and duration.
func (A Actor) VerifyDealsForActivation(rt Runtime, params *VerifyDealsForActivationParams) *VerifyDealsForActivationReturn {
	rt.ValidateImmediateCallerType(builtin.StorageMinerActorCodeID)
	minerAddr := rt.Message().Caller()

	var st State
	rt.State().Readonly(&st)
	store := adt.AsStore(rt)

	dealWeight, verifiedWeight, err := ValidateDealsForActivation(&st, store, params.DealIDs, minerAddr, params.SectorExpiry, params.SectorStart)
	builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to validate dealProposals for activation")

	return &VerifyDealsForActivationReturn{
		DealWeight:         dealWeight,
		VerifiedDealWeight: verifiedWeight,
	}
}

type ActivateDealsParams struct {
	DealIDs      []abi.DealID
	SectorExpiry abi.ChainEpoch
}

// Verify that a given set of storage deals is valid for a sector currently being ProveCommitted,
// update the market's internal state accordingly.
func (a Actor) ActivateDeals(rt Runtime, params *ActivateDealsParams) *adt.EmptyValue {
	rt.ValidateImmediateCallerType(builtin.StorageMinerActorCodeID)
	minerAddr := rt.Message().Caller()
	currEpoch := rt.CurrEpoch()

	var st State
	store := adt.AsStore(rt)

	// Update deal dealStates.
	rt.State().Transaction(&st, func() interface{} {
		_, _, err := ValidateDealsForActivation(&st, store, params.DealIDs, minerAddr, params.SectorExpiry, currEpoch)
		builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to validate dealProposals for activation")

		msm, err := st.mutator(adt.AsStore(rt)).withDealStates(WritePermission).
			withPendingProposals(ReadOnlyPermission).withDealProposals(ReadOnlyPermission).build()
		builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to load state")

		for _, dealID := range params.DealIDs {
			// This construction could be replaced with a single "update deal state" state method, possibly batched
			// over all deal ids at once.
			_, found, err := msm.dealStates.Get(dealID)
			builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to get state for dealId %d", dealID)
			if found {
				rt.Abortf(exitcode.ErrIllegalArgument, "deal %d already included in another sector", dealID)
			}

			proposal, err := getDealProposal(msm.dealProposals, dealID)
			builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to get dealId %d", dealID)

			propc, err := proposal.Cid()
			if err != nil {
				rt.Abortf(exitcode.ErrIllegalState, "get proposal cid %v", err)
			}

			has, err := msm.pendingDeals.Get(adt.CidKey(propc), nil)
			if err != nil {
				rt.Abortf(exitcode.ErrIllegalState, "no pending proposal for  %v", err)
			}

			if !has {
				rt.Abortf(exitcode.ErrIllegalState, "tried to active deal that was not in the pending set (%s)", propc)
			}

			err = msm.dealStates.Set(dealID, &DealState{
				SectorStartEpoch: currEpoch,
				LastUpdatedEpoch: epochUndefined,
				SlashEpoch:       epochUndefined,
			})
			builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to set deal state %d", dealID)
		}

		err = msm.commitState()
		builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to flush state")
		return nil
	})

	return nil
}

type ComputeDataCommitmentParams struct {
	DealIDs    []abi.DealID
	SectorType abi.RegisteredSealProof
}

func (a Actor) ComputeDataCommitment(rt Runtime, params *ComputeDataCommitmentParams) *cbg.CborCid {
	rt.ValidateImmediateCallerType(builtin.StorageMinerActorCodeID)

	pieces := make([]abi.PieceInfo, 0)
	var st State
	rt.State().Readonly(&st)
	proposals, err := AsDealProposalArray(adt.AsStore(rt), st.Proposals)
	builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to load deal dealProposals")

	for _, dealID := range params.DealIDs {
		deal, err := getDealProposal(proposals, dealID)
		builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to get dealId %d", dealID)

		pieces = append(pieces, abi.PieceInfo{
			PieceCID: deal.PieceCID,
			Size:     deal.PieceSize,
		})
	}

	commd, err := rt.Syscalls().ComputeUnsealedSectorCID(params.SectorType, pieces)
	if err != nil {
		rt.Abortf(exitcode.ErrIllegalArgument, "failed to compute unsealed sector CID: %s", err)
	}

	return (*cbg.CborCid)(&commd)
}

type OnMinerSectorsTerminateParams struct {
	Epoch   abi.ChainEpoch
	DealIDs []abi.DealID
}

// Terminate a set of deals in response to their containing sector being terminated.
// Slash provider collateral, refund client collateral, and refund partial unpaid escrow
// amount to client.
func (a Actor) OnMinerSectorsTerminate(rt Runtime, params *OnMinerSectorsTerminateParams) *adt.EmptyValue {
	rt.ValidateImmediateCallerType(builtin.StorageMinerActorCodeID)
	minerAddr := rt.Message().Caller()

	var st State
	rt.State().Transaction(&st, func() interface{} {
		msm, err := st.mutator(adt.AsStore(rt)).withDealStates(WritePermission).
			withDealProposals(ReadOnlyPermission).build()
		builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to load deal state")

		for _, dealID := range params.DealIDs {
			deal, found, err := msm.dealProposals.Get(dealID)
			if err != nil {
				rt.Abortf(exitcode.ErrIllegalState, "get deal: %v", err)
			}
			// deal could have terminated and hence deleted before the sector is terminated.
			// we should simply continue instead of aborting execution here if a deal is not found.
			if !found {
				continue
			}

			AssertMsg(deal.Provider == minerAddr, "caller is not the provider of the deal")

			// do not slash expired deals
			if deal.EndEpoch <= params.Epoch {
				continue
			}

			state, found, err := msm.dealStates.Get(dealID)
			if err != nil {
				rt.Abortf(exitcode.ErrIllegalState, "get deal: %v", err)
			}
			if !found {
				rt.Abortf(exitcode.ErrIllegalArgument, "no state found for deal in sector being terminated")
			}

			// if a deal is already slashed, we don't need to do anything here.
			if state.SlashEpoch != epochUndefined {
				continue
			}

			// mark the deal for slashing here.
			// actual releasing of locked funds for the client and slashing of provider collateral happens in CronTick.
			state.SlashEpoch = params.Epoch

			if err := msm.dealStates.Set(dealID, state); err != nil {
				rt.Abortf(exitcode.ErrIllegalState, "set deal: %v", err)
			}
		}

		err = msm.commitState()
		builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to flush state")

		return nil
	})
	return nil
}

func (a Actor) CronTick(rt Runtime, params *adt.EmptyValue) *adt.EmptyValue {
	rt.ValidateImmediateCallerIs(builtin.CronActorAddr)
	amountSlashed := big.Zero()

	var timedOutVerifiedDeals []*DealProposal

	var st State
	rt.State().Transaction(&st, func() interface{} {
		updatesNeeded := make(map[abi.ChainEpoch][]abi.DealID)

		msm, err := st.mutator(adt.AsStore(rt)).withDealStates(WritePermission).
			withLockedTable(WritePermission).withEscrowTable(WritePermission).withDealsByEpoch(WritePermission).
			withDealProposals(WritePermission).withPendingProposals(WritePermission).build()
		builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to load state")

		for i := st.LastCron + 1; i <= rt.CurrEpoch(); i++ {
			if err := msm.dealsByEpoch.ForEach(i, func(dealID abi.DealID) error {
				deal, err := getDealProposal(msm.dealProposals, dealID)
				builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to get dealId %d", dealID)

				dcid, err := deal.Cid()
				if err != nil {
					return xerrors.Errorf("failed to get cid for deal proposal: %w", err)
				}

				state, found, err := msm.dealStates.Get(dealID)
				builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to get deal state")

				// deal has been published but not activated yet -> terminate it as it has timed out
				if !found {
					// Not yet appeared in proven sector; check for timeout.
					AssertMsg(rt.CurrEpoch() >= deal.StartEpoch, "if sector start is not set, we must be in a timed out state")

					slashed := msm.processDealInitTimedOut(rt, deal)
					if !slashed.IsZero() {
						amountSlashed = big.Add(amountSlashed, slashed)
					}
					if deal.VerifiedDeal {
						timedOutVerifiedDeals = append(timedOutVerifiedDeals, deal)
					}

					// we should not attempt to delete the DealState because it does NOT exist
					if err := deleteDealProposalAndState(dealID, msm.dealStates, msm.dealProposals, true, false); err != nil {
						builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to delete deal")
					}

					// remove provider account if escrow balance is now zero.
					err, code := msm.removeAccountIfNoBalance(deal.Provider)
					builtin.RequireNoErr(rt, err, code, "failed to remove provider account")

					pdErr := msm.pendingDeals.Delete(adt.CidKey(dcid))
					builtin.RequireNoErr(rt, pdErr, exitcode.ErrIllegalState, "failed to delete pending proposal")

					return nil
				}

				// if this is the first cron tick for the deal, it should be in the pending state.
				if state.LastUpdatedEpoch == epochUndefined {
					pdErr := msm.pendingDeals.Delete(adt.CidKey(dcid))
					builtin.RequireNoErr(rt, pdErr, exitcode.ErrIllegalState, "failed to delete pending proposal")
				}

				slashAmount, nextEpoch, removeDeal := msm.updatePendingDealState(rt, state, deal, dealID, rt.CurrEpoch())
				if removeDeal {
					if err := deleteDealProposalAndState(dealID, msm.dealStates, msm.dealProposals, true, true); err != nil {
						builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to delete deal")
					}
				}
				if !slashAmount.IsZero() {
					amountSlashed = big.Add(amountSlashed, slashAmount)
				}

				if nextEpoch != epochUndefined {
					Assert(nextEpoch > rt.CurrEpoch())

					// TODO: can we avoid having this field?
					// https://github.com/filecoin-project/specs-actors/issues/463
					state.LastUpdatedEpoch = rt.CurrEpoch()

					if err := msm.dealStates.Set(dealID, state); err != nil {
						rt.Abortf(exitcode.ErrPlaceholder, "failed to set deal state state: %v", err)
					}

					updatesNeeded[nextEpoch] = append(updatesNeeded[nextEpoch], dealID)
				}

				// remove provider account if escrow balance is now zero
				err, code := msm.removeAccountIfNoBalance(deal.Provider)
				builtin.RequireNoErr(rt, err, code, "failed to remove provider account")

				// remove client account if escrow balance is now zero
				err, code = msm.removeAccountIfNoBalance(deal.Client)
				builtin.RequireNoErr(rt, err, code, "failed to remove client account")

				return nil
			}); err != nil {
				builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to iterate deals for epoch")
			}
			builtin.RequireNoErr(rt, msm.dealsByEpoch.RemoveAll(i), exitcode.ErrIllegalState, "failed to delete deals from set")
		}

		// Iterate changes in sorted order to ensure that loads/stores
		// are deterministic. Otherwise, we could end up charging an
		// inconsistent amount of gas.
		changedEpochs := make([]abi.ChainEpoch, 0, len(updatesNeeded))
		for epoch := range updatesNeeded { //nolint:nomaprange
			changedEpochs = append(changedEpochs, epoch)
		}

		sort.Slice(changedEpochs, func(i, j int) bool { return changedEpochs[i] < changedEpochs[j] })

		for _, epoch := range changedEpochs {
			if err := msm.dealsByEpoch.PutMany(epoch, updatesNeeded[epoch]); err != nil {
				rt.Abortf(exitcode.ErrIllegalState, "failed to reinsert deal IDs into epoch set: %s", err)
			}
		}

		st.LastCron = rt.CurrEpoch()

		err = msm.commitState()
		builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to flush state")

		return nil
	})

	for _, d := range timedOutVerifiedDeals {
		_, code := rt.Send(
			builtin.VerifiedRegistryActorAddr,
			builtin.MethodsVerifiedRegistry.RestoreBytes,
			&verifreg.RestoreBytesParams{
				Address:  d.Client,
				DealSize: big.NewIntUnsigned(uint64(d.PieceSize)),
			},
			abi.NewTokenAmount(0),
		)

		builtin.RequireSuccess(rt, code, "failed to restore bytes for verified client: %v", d.Client)
	}

	if !amountSlashed.IsZero() {
		_, e := rt.Send(builtin.BurntFundsActorAddr, builtin.MethodSend, nil, amountSlashed)
		builtin.RequireSuccess(rt, e, "expected send to burnt funds actor to succeed")
	}

	return nil
}

func deleteDealProposalAndState(dealId abi.DealID, states *DealMetaArray, proposals *DealArray, removeProposal bool,
	removeState bool) error {
	if removeProposal {
		if err := proposals.Delete(uint64(dealId)); err != nil {
			return fmt.Errorf("failed to delete deal proposal: %w", err)
		}

	}

	if removeState {
		if err := states.Delete(dealId); err != nil {
			return fmt.Errorf("failed to delete deal state: %w", err)
		}
	}

	return nil
}

//
// Exported functions
//

// Validates a collection of deal dealProposals for activation, and returns their combined weight,
// split into regular deal weight and verified deal weight.
func ValidateDealsForActivation(st *State, store adt.Store, dealIDs []abi.DealID, minerAddr addr.Address,
	sectorExpiry, currEpoch abi.ChainEpoch) (big.Int, big.Int, error) {

	proposals, err := AsDealProposalArray(store, st.Proposals)
	if err != nil {
		return big.Int{}, big.Int{}, fmt.Errorf("failed to load dealProposals: %w", err)
	}

	totalDealSpaceTime := big.Zero()
	totalVerifiedSpaceTime := big.Zero()
	for _, dealID := range dealIDs {
		proposal, found, err := proposals.Get(dealID)
		if err != nil {
			return big.Int{}, big.Int{}, fmt.Errorf("failed to load deal %d: %w", dealID, err)
		}
		if !found {
			return big.Int{}, big.Int{}, fmt.Errorf("dealId %d not found", dealID)
		}
		if err = validateDealCanActivate(proposal, minerAddr, sectorExpiry, currEpoch); err != nil {
			return big.Int{}, big.Int{}, fmt.Errorf("cannot activate deal %d: %w", dealID, err)
		}

		// Compute deal weight
		dealSpaceTime := DealWeight(proposal)
		if proposal.VerifiedDeal {
			totalVerifiedSpaceTime = big.Add(totalVerifiedSpaceTime, dealSpaceTime)
		} else {
			totalDealSpaceTime = big.Add(totalDealSpaceTime, dealSpaceTime)
		}
	}
	return totalDealSpaceTime, totalVerifiedSpaceTime, nil
}

////////////////////////////////////////////////////////////////////////////////
// Checks
////////////////////////////////////////////////////////////////////////////////

func validateDealCanActivate(proposal *DealProposal, minerAddr addr.Address, sectorExpiration, currEpoch abi.ChainEpoch) error {
	if proposal.Provider != minerAddr {
		return fmt.Errorf("proposal has provider %v, must be %v", proposal.Provider, minerAddr)
	}
	if currEpoch > proposal.StartEpoch {
		return fmt.Errorf("proposal start epoch %d has already elapsed at %d", proposal.StartEpoch, currEpoch)
	}
	if proposal.EndEpoch > sectorExpiration {
		return fmt.Errorf("proposal expiration %d exceeds sector expiration %d", proposal.EndEpoch, sectorExpiration)
	}
	return nil
}

func validateDeal(rt Runtime, deal ClientDealProposal) {
	if err := dealProposalIsInternallyValid(rt, deal); err != nil {
		rt.Abortf(exitcode.ErrIllegalArgument, "Invalid deal proposal: %s", err)
	}

	proposal := deal.Proposal

	if err := proposal.PieceSize.Validate(); err != nil {
		rt.Abortf(exitcode.ErrIllegalArgument, "proposal piece size is invalid: %v", err)
	}

	if !proposal.PieceCID.Defined() {
		rt.Abortf(exitcode.ErrIllegalArgument, "proposal PieceCID undefined")
	}

	if proposal.PieceCID.Prefix() != PieceCIDPrefix {
		rt.Abortf(exitcode.ErrIllegalArgument, "proposal PieceCID had wrong prefix")
	}

	if proposal.EndEpoch <= proposal.StartEpoch {
		rt.Abortf(exitcode.ErrIllegalArgument, "proposal end before proposal start")
	}

	if rt.CurrEpoch() > proposal.StartEpoch {
		rt.Abortf(exitcode.ErrIllegalArgument, "Deal start epoch has already elapsed.")
	}

	minDuration, maxDuration := dealDurationBounds(proposal.PieceSize)
	if proposal.Duration() < minDuration || proposal.Duration() > maxDuration {
		rt.Abortf(exitcode.ErrIllegalArgument, "Deal duration out of bounds.")
	}

	minPrice, maxPrice := dealPricePerEpochBounds(proposal.PieceSize, proposal.Duration())
	if proposal.StoragePricePerEpoch.LessThan(minPrice) || proposal.StoragePricePerEpoch.GreaterThan(maxPrice) {
		rt.Abortf(exitcode.ErrIllegalArgument, "Storage price out of bounds.")
	}

	minProviderCollateral, maxProviderCollateral := dealProviderCollateralBounds(proposal.PieceSize, proposal.Duration())
	if proposal.ProviderCollateral.LessThan(minProviderCollateral) || proposal.ProviderCollateral.GreaterThan(maxProviderCollateral) {
		rt.Abortf(exitcode.ErrIllegalArgument, "Provider collateral out of bounds.")
	}

	minClientCollateral, maxClientCollateral := dealClientCollateralBounds(proposal.PieceSize, proposal.Duration())
	if proposal.ClientCollateral.LessThan(minClientCollateral) || proposal.ClientCollateral.GreaterThan(maxClientCollateral) {
		rt.Abortf(exitcode.ErrIllegalArgument, "Client collateral out of bounds.")
	}
}

// Resolves a provider or client address to the canonical form against which a balance should be held, and
// the designated recipient address of withdrawals (which is the same, for simple account parties).
func escrowAddress(rt Runtime, address addr.Address) (nominal addr.Address, recipient addr.Address, approved []addr.Address) {
	// Resolve the provided address to the canonical form against which the balance is held.
	nominal, ok := rt.ResolveAddress(address)
	if !ok {
		rt.Abortf(exitcode.ErrIllegalArgument, "failed to resolve address %v", address)
	}

	codeID, ok := rt.GetActorCodeCID(nominal)
	if !ok {
		rt.Abortf(exitcode.ErrIllegalArgument, "no code for address %v", nominal)
	}

	if codeID.Equals(builtin.StorageMinerActorCodeID) {
		// Storage miner actor entry; implied funds recipient is the associated owner address.
		ownerAddr, workerAddr := builtin.RequestMinerControlAddrs(rt, nominal)
		return nominal, ownerAddr, []addr.Address{ownerAddr, workerAddr}
	}

	return nominal, nominal, []addr.Address{nominal}
}

func getDealProposal(proposals *DealArray, dealID abi.DealID) (*DealProposal, error) {
	proposal, found, err := proposals.Get(dealID)
	if err != nil {
		return nil, fmt.Errorf("failed to load proposal: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("deal %d not found", dealID)
	}

	return proposal, nil
}
