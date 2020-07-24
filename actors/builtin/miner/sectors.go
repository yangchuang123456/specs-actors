package miner

import (
	"fmt"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-cid"
)

func LoadSectors(store adt.Store, root cid.Cid) (Sectors, error) {
	sectorsArr, err := adt.AsArray(store, root)
	if err != nil {
		return Sectors{}, err
	}
	return Sectors{sectorsArr}, nil
}

// Sectors is a helper type for accessing/modifying a miner's sectors. It's safe
// to pass this object around as needed.
type Sectors struct {
	*adt.Array
}

func (sa Sectors) Load(sectorNos *abi.BitField) ([]*SectorOnChainInfo, error) {
	var sectorInfos []*SectorOnChainInfo
	if err := sectorNos.ForEach(func(i uint64) error {
		var sectorOnChain SectorOnChainInfo
		found, err := sa.Array.Get(i, &sectorOnChain)
		if err != nil {
			return xerrors.Errorf("failed to load sector %v: %w", abi.SectorNumber(i), err)
		} else if !found {
			return xerrors.Errorf("can't find sector %d", i)
		}
		sectorInfos = append(sectorInfos, &sectorOnChain)
		return nil
	}); err != nil {
		return nil, err
	}
	return sectorInfos, nil
}

func (sa Sectors) Get(sectorNumber abi.SectorNumber) (info *SectorOnChainInfo, found bool, err error) {
	var res SectorOnChainInfo
	if found, err := sa.Array.Get(uint64(sectorNumber), &res); err != nil {
		return nil, false, xerrors.Errorf("failed to get sector %d: %w", sectorNumber, err)
	} else if !found {
		return nil, false, nil
	}
	return &res, true, nil
}

func (sa Sectors) Store(infos ...*SectorOnChainInfo) error {
	for _, info := range infos {
		if info == nil {
			return xerrors.Errorf("nil sector info")
		}
		if info.SectorNumber > abi.MaxSectorNumber {
			return fmt.Errorf("sector number %d out of range", info.SectorNumber)
		}
		if err := sa.Set(uint64(info.SectorNumber), info); err != nil {
			return fmt.Errorf("failed to store sector %d: %w", info.SectorNumber, err)
		}
	}
	return nil
}

func (sa Sectors) MustGet(sectorNumber abi.SectorNumber) (info *SectorOnChainInfo, err error) {
	if info, found, err := sa.Get(sectorNumber); err != nil {
		return nil, err
	} else if !found {
		return nil, fmt.Errorf("sector %d not found", sectorNumber)
	} else {
		return info, nil
	}
}
